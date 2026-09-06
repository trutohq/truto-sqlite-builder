package sqlb

import (
	"fmt"
	"strings"
)

// The clause emitter.
//
// The reference builds a WHERE clause the way JavaScript makes convenient: each
// recursion returns a string, siblings are collected into an array, and the
// parent joins them. Transliterating that costs one allocation per node plus
// one per level — profiling a realistic three-level filter showed 166 of them,
// and 94% of the allocated bytes, in exactly that concatenate-and-join shape.
//
// So the compiler here appends into one growing buffer instead. The output is
// byte-identical; the differential harness in differential_test.go is what says
// so, across 100,000 generated filters.
//
// The one thing the buffer form loses is the ability to decide, after the fact,
// whether a node needs wrapping parentheses — the reference checks
// clauses.length. countClauses recovers it with a cheap allocation-free walk
// before emitting.

type compileCtx struct {
	dialect       Dialect
	depth         int
	operatorCount int
	values        []any
	buf           []byte
	// scratch holds the field expression while its operator clauses are emitted,
	// since a JSON path renders json_extract(…) once per clause.
	scratch []byte
}

// bind records a value and writes its bind marker.
//
// The marker is written AFTER the append so that a numbered dialect gets the
// position the value actually occupies. On the default `?` dialect the number
// is ignored and this is the same byte it always was.
func (c *compileCtx) bind(v any) {
	c.values = append(c.values, v)
	c.buf = c.dialect.appendPlaceholder(c.buf, len(c.values))
}

func (c *compileCtx) countOperator() error {
	c.operatorCount++
	if c.operatorCount > MaxOperators {
		return fmt.Errorf("Too many operators (max: %d)", MaxOperators)
	}
	return nil
}

// CompileFilter compiles a filter to a parenthesised WHERE clause with its
// bound values. Port of compileFilter (src/filter.ts:417).
//
// It compiles for SQLite; use [Dialect.CompileWhere] for another database.
func CompileFilter(f *Filter) (Fragment, error) {
	clause, err := SQLite.CompileWhere(f)
	if err != nil {
		return Fragment{}, err
	}
	return Fragment{text: clause.text, values: clause.values}, nil
}

// Clause is a compiled WHERE clause and the values bound to it, in placeholder
// order. It is what [Dialect.CompileWhere] returns.
//
// It is deliberately NOT a [Fragment], and that is the whole point of having a
// second type. A Postgres or MSSQL placeholder carries its absolute position in
// its own text — `$1`, `@p1` — so splicing two independently compiled clauses
// into one statement would need renumbering, and a renumbering that goes wrong
// binds every value to the wrong column while producing SQL that still runs.
// Rather than police that at run time, a Clause simply has no way into [Q],
// [Join] or [Idents]: the caller assembles the statement, which is what Envoy's
// connectors already do (e.g. pgconn/postgres.go:148-170).
type Clause struct {
	text    string
	values  []any
	dialect Dialect
}

// Text returns the WHERE clause, without the WHERE keyword.
func (c Clause) Text() string { return c.text }

// Values returns the bound values in placeholder order. The slice aliases the
// clause's own storage and must not be modified.
func (c Clause) Values() []any { return c.values }

// Dialect returns the dialect the clause was compiled for.
func (c Clause) Dialect() Dialect { return c.dialect }

// IsEmpty reports that the filter had no conditions, so there is no WHERE
// clause to write.
//
// Check it before assembling a statement, and check it TWICE on a destructive
// one — an empty clause on a DELETE or UPDATE affects every row, and since
// D-0032 the library no longer refuses on your behalf:
//
//	if clause.IsEmpty() && destructive {
//	    return fmt.Errorf("refusing to delete with no filter")
//	}
//	q := "SELECT * FROM t"
//	if !clause.IsEmpty() {
//	    q += " WHERE " + clause.Text()
//	}
//
// The zero Clause — what [Dialect.CompileWhere] returns alongside an error — is
// also empty, so handle the error before asking. Treating a failed compile as
// "no conditions" is the same outage from the other direction.
func (c Clause) IsEmpty() bool { return c.text == "" }

// CompileWhere is [CompileFilter] in this dialect's SQL.
//
// Everything that decides which ROWS match — operator semantics, clause order,
// the truthiness of `exists`, what an empty filter means, the identifier
// charset — is identical across dialects and is the reference's. Only the
// spelling changes: the bind marker, the identifier quote, and how `ilike`,
// `regex` and a JSON path are written.
func (d Dialect) CompileWhere(f *Filter) (Clause, error) {
	d = d.resolve()

	// A nil *Filter is the reference's `compileFilter(null)` and still errors.
	// Only an EMPTY one is "no conditions".
	if f == nil {
		return Clause{}, errFilterNotObject
	}

	// D-0032: a filter with no conditions compiles to NO WHERE CLAUSE — an empty
	// Clause and a nil error — rather than [ErrEmptyFilter]. The reference throws
	// here (src/filter.ts:403); this is the one place sqlb deliberately answers a
	// different question, because "no filter" meaning "every row" is what a
	// programmer expects and what goenvoy/internal/sqlfilter:83 already does.
	//
	// The danger is entirely on the caller's side and it is not theoretical: the
	// same empty clause on a DELETE or an UPDATE means every row in the table.
	// The library will no longer refuse for you. See [Clause.IsEmpty] and the
	// "empty filter" section of ENVOY-MIGRATION.md.
	//
	// Only the TOP level is affected. A nested empty filter — `{"and":[{}]}`, or
	// an empty alias block — still returns [ErrEmptyFilter], because emitting
	// nothing there would either produce `()` or silently drop a conjunct, and
	// because the reference's error is the useful answer for a filter that was
	// written but says nothing.
	if countClauses(f) == 0 {
		return Clause{dialect: d}, nil
	}

	ctx := compileCtx{
		dialect: d,
		buf:     make([]byte, 0, 256),
		values:  make([]any, 0, 16),
	}
	ctx.buf = append(ctx.buf, '(')
	if err := compileFilterInto(f, &ctx, ""); err != nil {
		return Clause{}, err
	}
	ctx.buf = append(ctx.buf, ')')

	text := string(ctx.buf)

	// Backstop against output-size denial of service: the identifier and
	// operator limits bound each piece, this bounds the aggregate for callers
	// using CompileFilter standalone. src/filter.ts:430.
	if length, over := exceedsUTF16(text, MaxQueryLength); over {
		return Clause{}, fmt.Errorf("Compiled filter too long: %d bytes (max: %d)",
			length, MaxQueryLength)
	}

	return Clause{text: text, values: ctx.values, dialect: d}, nil
}

// countClauses reports how many clauses a filter node will emit, so the emitter
// knows whether to wrap. It must agree exactly with compileFilterInto's own
// skipping rules, and it must not allocate.
func countClauses(f *Filter) int {
	n := 0
	if v, ok := f.get("and"); ok && jsTruthy(v) {
		n++
	}
	if v, ok := f.get("or"); ok && jsTruthy(v) {
		n++
	}
	for _, e := range f.entries {
		if e.key == "and" || e.key == "or" || strings.HasPrefix(e.key, "$") {
			continue
		}
		if _, undef := e.value.(undefinedT); undef {
			continue
		}
		n++
	}
	for _, e := range f.entries {
		if strings.HasPrefix(e.key, "$") && isFilterLike(e.value) {
			n++
		}
	}
	return n
}

func compileFilterInto(f *Filter, ctx *compileCtx, alias string) error {
	if ctx.depth >= MaxNestingDepth {
		return fmt.Errorf("Nesting depth too deep (max: %d)", MaxNestingDepth)
	}
	if f == nil {
		return errFilterNotObject
	}

	total := countClauses(f)
	if total == 0 {
		return errFilterEmpty
	}

	ctx.depth++
	if total > 1 {
		ctx.buf = append(ctx.buf, '(')
	}
	written := 0

	emitSeparator := func() {
		if written > 0 {
			ctx.buf = append(ctx.buf, " AND "...)
		}
		written++
	}

	// Logical operators first, `and` before `or`, whatever order the keys were
	// written in (src/filter.ts:314-350).
	for _, logical := range [...]struct {
		key, sqlOp, upper string
	}{
		{"and", " AND ", "AND"},
		{"or", " OR ", "OR"},
	} {
		value, present := f.get(logical.key)
		if !present || !jsTruthy(value) {
			continue
		}
		if err := ctx.countOperator(); err != nil {
			return err
		}
		subs, ok := asFilterSlice(value)
		if !ok {
			return fmt.Errorf("%s operator must be an array", logical.upper)
		}
		if len(subs) == 0 {
			return fmt.Errorf("%s operator cannot be used with empty arrays", logical.upper)
		}
		emitSeparator()
		ctx.buf = append(ctx.buf, '(')
		for i, sub := range subs {
			if i > 0 {
				ctx.buf = append(ctx.buf, logical.sqlOp...)
			}
			// A non-object element is passed down rather than rejected here: the
			// reference calls compileFilterRecursive on it, which checks the DEPTH
			// limit before the type, so at maximum depth the depth error wins.
			if err := compileFilterInto(sub, ctx, alias); err != nil {
				return err
			}
		}
		ctx.buf = append(ctx.buf, ')')
	}

	// Field conditions, in the filter's own order (src/filter.ts:353-372).
	for _, e := range f.entries {
		if e.key == "and" || e.key == "or" || strings.HasPrefix(e.key, "$") {
			continue
		}
		if _, undef := e.value.(undefinedT); undef {
			continue
		}
		if isJSArray(e.value) {
			return fmt.Errorf(
				"Field '%s' cannot have array value. Use logical operators 'and'/'or' instead.", e.key)
		}
		emitSeparator()
		if err := compileFieldConditionInto(e.key, e.value, ctx, alias); err != nil {
			return err
		}
	}

	// Alias blocks last (src/filter.ts:375-398).
	for _, e := range f.entries {
		if !strings.HasPrefix(e.key, "$") {
			continue
		}
		sub, ok := asFilterLike(e.value)
		if !ok {
			// The reference requires typeof value === 'object' && value !== null, so
			// undefined, null and scalars fall out of both lists silently.
			continue
		}
		name := e.key[1:]
		if !isValidAlias(name) {
			return fmt.Errorf("Invalid alias identifier: %s", name)
		}
		emitSeparator()
		if err := compileFilterInto(sub, ctx, name); err != nil {
			return err
		}
	}

	if total > 1 {
		ctx.buf = append(ctx.buf, ')')
	}
	ctx.depth--
	return nil
}

// isValidAlias is isValidSqlIdentifier (src/filter.ts:77). The alias is emitted
// UNQUOTED as `alias."column"`, so this is the only thing standing between a
// caller and raw SQL.
func isValidAlias(name string) bool {
	return len(name) <= MaxIdentifierLength && isSimpleIdent(name)
}

// compileFieldConditionInto compiles one field. Port of src/filter.ts:99.
func compileFieldConditionInto(field string, condition any, ctx *compileCtx, alias string) error {
	if scalar, isScalar := asScalar(condition); isScalar {
		if err := ctx.countOperator(); err != nil {
			return err
		}

		column, path, isPath, err := splitJSONPath(field)
		if err != nil {
			return err
		}
		if err := validateIdent(column); err != nil {
			return err
		}
		if isPath && !ctx.dialect.SupportsJSONPath() {
			return unsupportedJSONPath(field)
		}

		ctx.buf = append(ctx.buf, '(')
		if isPath {
			ctx.buf = append(ctx.buf, ctx.dialect.jsonExtract...)
			ctx.buf = append(ctx.buf, '(')
		}
		ctx.buf = appendQualified(ctx.buf, alias, column, ctx.dialect.quote)
		if isPath {
			ctx.buf = append(ctx.buf, ", "...)
			ctx.bind(path)
			ctx.buf = append(ctx.buf, ')')
		}

		if scalar == nil {
			ctx.buf = append(ctx.buf, " IS NULL)"...)
			return nil
		}
		v, err := Value(scalar)
		if err != nil {
			return err
		}
		ctx.buf = append(ctx.buf, " = "...)
		ctx.bind(v)
		ctx.buf = append(ctx.buf, ')')
		return nil
	}

	ops, ok := asOps(condition)
	if !ok {
		return errConditionType
	}

	for _, e := range ops.entries {
		if !validOperators[e.name] {
			return fmt.Errorf("Unknown operator: %s", e.name)
		}
	}

	column, path, isPath, err := splitJSONPath(field)
	if err != nil {
		return err
	}
	if err := validateIdent(column); err != nil {
		return err
	}
	if isPath && !ctx.dialect.SupportsJSONPath() {
		return unsupportedJSONPath(field)
	}

	ctx.scratch = ctx.scratch[:0]
	if isPath {
		ctx.scratch = append(ctx.scratch, ctx.dialect.jsonExtract...)
		ctx.scratch = append(ctx.scratch, '(')
	}
	ctx.scratch = appendQualified(ctx.scratch, alias, column, ctx.dialect.quote)
	if isPath {
		// The path is bound ONCE even though the expression may be rendered into
		// several clauses. On the `?` dialects that is the reference's
		// misalignment (src/filter.ts:169-176), preserved deliberately; see
		// TestFilter_JSONPathMultiOperatorMisalignment.
		//
		// A NUMBERED dialect does not have the bug, and not because anything was
		// fixed: $1 repeated in three clauses refers to the same bound value, so
		// binding once is already right. Same code, different spelling, different
		// correctness — which is why the misalignment test is SQLite-only.
		ctx.scratch = append(ctx.scratch, ", "...)
		ctx.values = append(ctx.values, path)
		ctx.scratch = ctx.dialect.appendPlaceholder(ctx.scratch, len(ctx.values))
		ctx.scratch = append(ctx.scratch, ')')
	}
	fieldExpr := ctx.scratch

	// exists short-circuits the whole condition regardless of its position among
	// the operators (src/filter.ts:179).
	if existsValue, present := ops.get("exists"); present {
		if err := ctx.countOperator(); err != nil {
			return err
		}
		ctx.buf = append(ctx.buf, '(')
		ctx.buf = append(ctx.buf, fieldExpr...)
		if jsTruthy(existsValue) {
			ctx.buf = append(ctx.buf, " IS NOT NULL)"...)
		} else {
			ctx.buf = append(ctx.buf, " IS NULL)"...)
		}
		return nil
	}

	// The reference wraps the operator list in parentheses whether there is one
	// clause or several (src/filter.ts:291), so no count is needed here — but the
	// open paren has to be written before we know whether ANY clause follows, and
	// an operator object with no emitting operator is an error. Remember where it
	// went so it can be unwound.
	openAt := len(ctx.buf)
	ctx.buf = append(ctx.buf, '(')
	written := 0

	for _, e := range ops.entries {
		if err := ctx.countOperator(); err != nil {
			return err
		}
		if !emits(e.name) {
			// "and" and "or" are valid operator names with no case in the
			// reference's switch, so they contribute nothing. src/filter.ts:197.
			continue
		}
		if written > 0 {
			ctx.buf = append(ctx.buf, " AND "...)
		}
		written++
		if err := compileOperatorInto(e.name, e.value, fieldExpr, ctx); err != nil {
			return err
		}
	}

	if written == 0 {
		ctx.buf = ctx.buf[:openAt]
		return errNoValidOperator
	}
	ctx.buf = append(ctx.buf, ')')
	return nil
}

func emits(op string) bool { return op != "and" && op != "or" }

func compileOperatorInto(op string, value any, fieldExpr []byte, ctx *compileCtx) error {
	switch op {
	case "gt", "gte", "lt", "lte":
		v, err := Value(value)
		if err != nil {
			return err
		}
		ctx.buf = append(ctx.buf, fieldExpr...)
		ctx.buf = append(ctx.buf, ' ')
		ctx.buf = append(ctx.buf, comparator(op)...)
		ctx.buf = append(ctx.buf, ' ')
		ctx.bind(v)
		return nil

	case "ne":
		ctx.buf = append(ctx.buf, fieldExpr...)
		if isNullish(value) {
			ctx.buf = append(ctx.buf, " IS NOT NULL"...)
			return nil
		}
		v, err := Value(value)
		if err != nil {
			return err
		}
		ctx.buf = append(ctx.buf, " <> "...)
		ctx.bind(v)
		return nil

	case "in", "nin":
		upper := "IN"
		if op == "nin" {
			upper = "NIN"
		}
		items, ok := asJSArray(value)
		if !ok {
			return fmt.Errorf("%s operator requires an array", upper)
		}
		if len(items) == 0 {
			return fmt.Errorf("%s operator cannot be used with empty arrays", upper)
		}
		if len(items) > 999 {
			return fmt.Errorf("%s operator cannot be used with arrays larger than 999 items", upper)
		}
		// The reference builds a fragment with sql.in() and copies its values
		// across; the placeholder text is a function of the length alone, so it is
		// emitted directly and only the conversion is kept.
		ctx.buf = append(ctx.buf, fieldExpr...)
		if op == "nin" {
			ctx.buf = append(ctx.buf, " NOT"...)
		}
		ctx.buf = append(ctx.buf, " IN ("...)
		for i, item := range items {
			if i > 0 {
				ctx.buf = append(ctx.buf, ',')
			}
			v, err := Value(item)
			if err != nil {
				return err
			}
			ctx.bind(v)
		}
		ctx.buf = append(ctx.buf, ')')
		return nil

	case "like", "ilike", "regex":
		name := "LIKE"
		switch op {
		case "ilike":
			name = "ILIKE"
		case "regex":
			name = "REGEX"
		}
		pattern, ok := asString(value)
		if !ok {
			return fmt.Errorf("%s operator requires a string pattern", name)
		}
		if length, over := exceedsUTF16(pattern, MaxPatternLength); over {
			return fmt.Errorf("%s pattern too long: %d characters (max: %d)",
				name, length, MaxPatternLength)
		}
		clause := opClause{mid: " LIKE "}
		switch op {
		case "ilike":
			clause = ctx.dialect.ilike
		case "regex":
			clause = ctx.dialect.regex
		}
		// A dialect that cannot spell the operator errors rather than emitting
		// the nearest thing. T-SQL has no REGEXP, and silently downgrading it to
		// LIKE would return a different set of rows with no sign that it had.
		if !clause.supported() {
			return fmt.Errorf("%s operator is not supported by the %s dialect",
				name, ctx.dialect.name)
		}
		ctx.buf = append(ctx.buf, clause.pre...)
		ctx.buf = append(ctx.buf, fieldExpr...)
		ctx.buf = append(ctx.buf, clause.mid...)
		ctx.bind(pattern)
		ctx.buf = append(ctx.buf, clause.post...)
		return nil
	}
	return nil
}

// unsupportedJSONPath is sqlfilter's message verbatim (sqlfilter.go:116), so a
// task that already handles it keeps matching after the swap.
func unsupportedJSONPath(field string) error {
	return fmt.Errorf("JSON path filters are not supported by this database: %s", field)
}

func comparator(op string) string {
	switch op {
	case "gt":
		return ">"
	case "gte":
		return ">="
	case "lt":
		return "<"
	default:
		return "<="
	}
}

// appendQualified writes `alias."column"`, or just `"column"` when there is no
// alias. The alias is deliberately NOT quoted, matching src/filter.ts:122-124;
// isValidAlias is what makes that safe. That holds in every dialect: the alias
// is a bare [A-Za-z_][A-Za-z0-9_]* in all of them.
func appendQualified(buf []byte, alias, column string, q quoteStyle) []byte {
	if alias != "" {
		buf = append(buf, alias...)
		buf = append(buf, '.')
	}
	return appendQuotedIdent(buf, column, q)
}

// splitJSONPath implements isJsonPath + compileJsonPath (src/filter.ts:49-72)
// without splitting into a slice.
func splitJSONPath(field string) (column, path string, isPath bool, err error) {
	dot := strings.IndexByte(field, '.')
	if dot < 0 {
		return field, "", false, nil
	}
	column = field[:dot]
	rest := field[dot+1:]
	if column == "" || rest == "" {
		return "", "", false, fmt.Errorf("Invalid JSON path: %s", field)
	}
	// Every dot-separated segment of the tail must be non-empty.
	start := 0
	for i := 0; i <= len(rest); i++ {
		if i != len(rest) && rest[i] != '.' {
			continue
		}
		if i == start {
			return "", "", false, fmt.Errorf("Invalid JSON path: %s", field)
		}
		start = i + 1
	}
	return column, "$." + rest, true, nil
}
