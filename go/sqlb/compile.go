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
	depth         int
	operatorCount int
	values        []any
	buf           []byte
	// scratch holds the field expression while its operator clauses are emitted,
	// since a JSON path renders json_extract(…) once per clause.
	scratch []byte
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
func CompileFilter(f *Filter) (Fragment, error) {
	ctx := compileCtx{
		buf:    make([]byte, 0, 256),
		values: make([]any, 0, 16),
	}
	ctx.buf = append(ctx.buf, '(')
	if err := compileFilterInto(f, &ctx, ""); err != nil {
		return Fragment{}, err
	}
	ctx.buf = append(ctx.buf, ')')

	text := string(ctx.buf)

	// Backstop against output-size denial of service: the identifier and
	// operator limits bound each piece, this bounds the aggregate for callers
	// using CompileFilter standalone. src/filter.ts:430.
	if length, over := exceedsUTF16(text, MaxQueryLength); over {
		return Fragment{}, fmt.Errorf("Compiled filter too long: %d bytes (max: %d)",
			length, MaxQueryLength)
	}

	return Fragment{text: text, values: ctx.values}, nil
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

		ctx.buf = append(ctx.buf, '(')
		if isPath {
			ctx.buf = append(ctx.buf, "json_extract("...)
		}
		ctx.buf = appendQualified(ctx.buf, alias, column)
		if isPath {
			ctx.buf = append(ctx.buf, ", ?)"...)
			ctx.values = append(ctx.values, path)
		}

		if scalar == nil {
			ctx.buf = append(ctx.buf, " IS NULL)"...)
			return nil
		}
		v, err := Value(scalar)
		if err != nil {
			return err
		}
		ctx.values = append(ctx.values, v)
		ctx.buf = append(ctx.buf, " = ?)"...)
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

	ctx.scratch = ctx.scratch[:0]
	if isPath {
		ctx.scratch = append(ctx.scratch, "json_extract("...)
	}
	ctx.scratch = appendQualified(ctx.scratch, alias, column)
	if isPath {
		ctx.scratch = append(ctx.scratch, ", ?)"...)
		// The path is bound ONCE even though the expression may be rendered into
		// several clauses. That misalignment is the reference's
		// (src/filter.ts:169-176) and is preserved; see
		// TestFilter_JSONPathMultiOperatorMisalignment.
		ctx.values = append(ctx.values, path)
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
		ctx.values = append(ctx.values, v)
		ctx.buf = append(ctx.buf, fieldExpr...)
		ctx.buf = append(ctx.buf, ' ')
		ctx.buf = append(ctx.buf, comparator(op)...)
		ctx.buf = append(ctx.buf, " ?"...)
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
		ctx.values = append(ctx.values, v)
		ctx.buf = append(ctx.buf, " <> ?"...)
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
			ctx.buf = append(ctx.buf, '?')
			v, err := Value(item)
			if err != nil {
				return err
			}
			ctx.values = append(ctx.values, v)
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
		ctx.values = append(ctx.values, pattern)
		ctx.buf = append(ctx.buf, fieldExpr...)
		switch op {
		case "like":
			ctx.buf = append(ctx.buf, " LIKE ?"...)
		case "ilike":
			ctx.buf = append(ctx.buf, " LIKE ? COLLATE NOCASE"...)
		default:
			ctx.buf = append(ctx.buf, " REGEXP ?"...)
		}
		return nil
	}
	return nil
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
// isValidAlias is what makes that safe.
func appendQualified(buf []byte, alias, column string) []byte {
	if alias != "" {
		buf = append(buf, alias...)
		buf = append(buf, '.')
	}
	return appendQuotedIdent(buf, column)
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
