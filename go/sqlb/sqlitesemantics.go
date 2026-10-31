package sqlb

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// An Option changes what a dialect's operators MEAN, where [Dialect] alone
// only changes how they are spelled. Options exist for one consumer's case:
// a program that moved its data from SQLite to Postgres and must keep
// answering its clients exactly as SQLite did (Truto, D-0873). Envoy's
// [Postgres] is the dialect without options and is unchanged by them.
type Option interface {
	apply(d *Dialect) error
}

type flagOption uint8

const (
	flagLike flagOption = 1 << iota
	flagJSONPath
)

func (f flagOption) apply(d *Dialect) error {
	switch f {
	case flagLike:
		d.sqliteLike = true
		d.regex = opClause{}
	case flagJSONPath:
		d.sqliteJSON = true
	default:
		return fmt.Errorf("sqlb: unknown option %d", f)
	}
	return nil
}

var (
	// SQLiteLike makes `like` and `ilike` answer as SQLite's LIKE does:
	// case-insensitive for ASCII letters only, with no escape character, over
	// SQLite's text of the operand (an INTEGER is its digits, a BOOLEAN 1 or
	// 0). On Postgres both render as
	//
	//	(<SQLite's text of x>) COLLATE "C" ILIKE ? ESCAPE ''
	//
	// `COLLATE "C"` is what makes the case folding ASCII-only, whatever the
	// database's own locale (measured on Postgres 17: 'É' ILIKE 'é' is false
	// under "C"). `regex` is refused, as D1 refuses it (no REGEXP function).
	//
	// Not reproduced: a REAL's text (SQLite writes 5.0 as '5.0', Postgres
	// '5'), and a date-time's text (whatever the column type renders).
	SQLiteLike Option = flagLike

	// SQLiteJSONPath makes a dotted field (`config.a.b`) answer as SQLite's
	// `json_extract(config, '$.a.b')` does, compared under SQLite's rules:
	// JSON null and a missing path are NULL; true and false are the integers
	// 1 and 0; numbers compare numerically (an integer literal exactly, a
	// real one as a double); strings, objects and arrays are text and compare
	// byte-wise; and across storage classes NULL < numbers < text < blob, so
	// `config.a > 5` is TRUE for every string. Sorting follows the same order.
	//
	// Numbers are bound as D1 binds them, as REALs: a Go integer compares as
	// the double it converts to, and NaN and ±Inf bind as NULL.
	SQLiteJSONPath Option = flagJSONPath
)

// With returns the dialect with the options applied.
//
// On [SQLite] the options describe what it already does, so it is returned
// unchanged. On [Postgres] they change the rendering as each option says. On
// any other dialect they are an error: nothing renders them there, and a
// silent no-op would answer different rows than the caller asked for.
func (d Dialect) With(opts ...Option) (Dialect, error) {
	d = d.resolve()
	if d.name == SQLite.name {
		return d, nil
	}
	if d.name != Postgres.name {
		return Dialect{}, fmt.Errorf("sqlb: the %s dialect has no SQLite-semantics options", d.name)
	}
	for _, o := range opts {
		if o == nil {
			return Dialect{}, errors.New("sqlb: a nil option")
		}
		if err := o.apply(&d); err != nil {
			return Dialect{}, err
		}
	}
	return d, nil
}

// --- SQLite's JSON path ------------------------------------------------------

type stepKind uint8

const (
	stepKey     stepKind = iota // .key  — matches only inside an object
	stepIndex                   // [n]   — matches only inside an array
	stepFromEnd                 // [#-n] — the n-th element from the end
	stepMiss                    // [#], or an index past any array: never found
)

type pathStep struct {
	kind  stepKind
	key   string
	index int64
}

// errBadJSONPath is SQLite's message (json.c, jsonBadPathError).
func errBadJSONPath(path string) error {
	return fmt.Errorf("bad JSON path: '%s'", path)
}

// parseSQLitePath reads a JSON path as SQLite's json_extract reads it
// (json.c jsonLookupStep, SQLite 3.45 and later as modernc 1.57 embeds it):
//
//   - `$` starts it; then each step is `.key`, `."quoted key"` or `[n]`,
//     `[#]`, `[#-n]`;
//   - an unquoted key runs to the next `.` or `[`, and may not be empty;
//   - a quoted key runs to the next unescaped `"`, its escapes are JSON's;
//   - `.key` on an array and `[n]` on an object find nothing, which is why the
//     steps keep their kind instead of collapsing to Postgres's text[] path
//     (where `{a,0}` indexes an array AND names an object key).
//
// # One difference, by design
//
// SQLite discovers a malformed step only when a row's document reaches it:
// `$.a[x]` is an error for a row whose `a` is an array and NULL for every
// other row. This refuses the path up front, so a statement whose rows never
// reach the bad step is an error here and an answer on SQLite.
func parseSQLitePath(path string) ([]pathStep, error) {
	if !strings.HasPrefix(path, "$") {
		return nil, errBadJSONPath(path)
	}
	var steps []pathStep
	i, n := 1, len(path)
	for i < n {
		switch path[i] {
		case '.':
			i++
			if i < n && path[i] == '"' {
				j := i + 1
				for j < n && path[j] != '"' {
					if path[j] == '\\' && j+1 < n {
						j++
					}
					j++
				}
				if j >= n {
					return nil, errBadJSONPath(path)
				}
				key, ok := unescapeJSONKey(path[i+1 : j])
				if !ok {
					return nil, errBadJSONPath(path)
				}
				steps = append(steps, pathStep{kind: stepKey, key: key})
				i = j + 1
				continue
			}
			j := i
			for j < n && path[j] != '.' && path[j] != '[' {
				j++
			}
			if j == i {
				return nil, errBadJSONPath(path)
			}
			steps = append(steps, pathStep{kind: stepKey, key: path[i:j]})
			i = j
		case '[':
			j := i + 1
			var k uint64
			for j < n && path[j] >= '0' && path[j] <= '9' {
				if k < 0xffffffff {
					k = k*10 + uint64(path[j]-'0')
				}
				j++
			}
			if j > i+1 && j < n && path[j] == ']' {
				step := pathStep{kind: stepIndex, index: int64(k)}
				if k > math.MaxInt32 {
					step = pathStep{kind: stepMiss}
				}
				steps = append(steps, step)
				i = j + 1
				continue
			}
			if i+1 < n && path[i+1] == '#' {
				j = i + 2
				step := pathStep{kind: stepMiss}
				if j+1 < n && path[j] == '-' && path[j+1] >= '0' && path[j+1] <= '9' {
					var m uint64
					j++
					for j < n && path[j] >= '0' && path[j] <= '9' {
						if m < 0xffffffff {
							m = m*10 + uint64(path[j]-'0')
						}
						j++
					}
					if m > 0 && m <= math.MaxInt32 {
						step = pathStep{kind: stepFromEnd, index: int64(m)}
					}
				}
				if j >= n || path[j] != ']' {
					return nil, errBadJSONPath(path)
				}
				steps = append(steps, step)
				i = j + 1
				continue
			}
			return nil, errBadJSONPath(path)
		default:
			return nil, errBadJSONPath(path)
		}
	}
	return steps, nil
}

// unescapeJSONKey decodes the escapes of a quoted path key, which SQLite
// compares against the document's keys after decoding both (jsonLabelCompare).
func unescapeJSONKey(s string) (string, bool) {
	if !strings.Contains(s, `\`) {
		return s, true
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(s) {
			return "", false
		}
		switch s[i] {
		case '"', '\\', '/':
			b.WriteByte(s[i])
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			if i+4 >= len(s) {
				return "", false
			}
			v, err := strconv.ParseUint(s[i+1:i+5], 16, 16)
			if err != nil {
				return "", false
			}
			r := rune(v)
			i += 4
			if utf16.IsSurrogate(r) && i+6 < len(s) && s[i+1] == '\\' && s[i+2] == 'u' {
				if lo, err := strconv.ParseUint(s[i+3:i+7], 16, 16); err == nil {
					if p := utf16.DecodeRune(r, rune(lo)); p != utf8.RuneError {
						r = p
						i += 6
					}
				}
			}
			b.WriteRune(r)
		default:
			return "", false
		}
	}
	return b.String(), true
}

// --- rendering -----------------------------------------------------------------

// jsonExpr is the JSON value a comparison reads: either a path into a JSON
// column (`json_extract(col, path)`) or a plain column seen through to_json,
// which is how SQLite's text of ANY column type is reached without knowing
// its type (SQLiteLike).
type jsonExpr struct {
	col    []byte
	steps  []pathStep
	toJSON bool

	// ref is the expression as the scope [compileCtx.scoped] names it.
	ref bool
}

// scopeRef is what an expression is called inside its scope.
var scopeRef = jsonExpr{ref: true}

// scoped writes `(SELECT <body> FROM (SELECT <e> AS j) AS sqlb_j)`: the JSON
// value is computed, and its keys bound, ONCE per comparison, while the body
// reads it as often as SQLite's rules need (a dozen times for a numeric
// comparison). Inline, a 999-value `in` over a two-step path would bind ~45,000
// values, near Postgres's 65,535; scoped it binds ~4,000. Postgres plans the
// scope as a one-row Result per row, not a scan (measured, EXPLAIN on 17).
//
// The body is written first and the scope after, which is also the order
// their `?` markers appear in, so binding as we write keeps values aligned.
func (c *compileCtx) scoped(e jsonExpr, body func(j jsonExpr) error) error {
	c.buf = append(c.buf, "(SELECT "...)
	if err := body(scopeRef); err != nil {
		return err
	}
	c.buf = append(c.buf, " FROM (SELECT "...)
	c.writeJ(e)
	c.buf = append(c.buf, " AS j) AS sqlb_j)"...)
	return nil
}

// writeJ writes the json-typed expression. Each key and index is bound.
func (c *compileCtx) writeJ(e jsonExpr) {
	if e.ref {
		c.buf = append(c.buf, "sqlb_j.j"...)
		return
	}
	if e.toJSON {
		c.buf = append(c.buf, "to_json("...)
		c.buf = append(c.buf, e.col...)
		c.buf = append(c.buf, ')')
		return
	}
	for _, s := range e.steps {
		if s.kind == stepMiss {
			c.buf = append(c.buf, "CAST(NULL AS json)"...)
			return
		}
	}
	c.buf = append(c.buf, "(CAST("...)
	c.buf = append(c.buf, e.col...)
	c.buf = append(c.buf, " AS json)"...)
	for _, s := range e.steps {
		switch s.kind {
		case stepKey:
			// json -> text is a key lookup only: on an array it is NULL,
			// which is SQLite's `.key` on an array.
			c.buf = append(c.buf, " -> CAST("...)
			c.bind(s.key)
			c.buf = append(c.buf, " AS text)"...)
		case stepIndex:
			// json -> integer is an index only: on an object it is NULL.
			c.buf = append(c.buf, " -> CAST("...)
			c.bind(s.index)
			c.buf = append(c.buf, " AS integer)"...)
		case stepFromEnd:
			c.buf = append(c.buf, " -> CAST("...)
			c.bind(-s.index)
			c.buf = append(c.buf, " AS integer)"...)
		}
	}
	c.buf = append(c.buf, ')')
}

// writeJText is `J #>> '{}'`: a string's text, a number's literal, an
// object's or array's JSON text.
func (c *compileCtx) writeJText(e jsonExpr) {
	c.buf = append(c.buf, '(')
	c.writeJ(e)
	c.buf = append(c.buf, " #>> '{}')"...)
}

// writeJIsNull is SQLite's json_extract(...) IS NULL: JSON null or no value.
func (c *compileCtx) writeJClass(e jsonExpr) {
	c.buf = append(c.buf, "COALESCE(json_typeof("...)
	c.writeJ(e)
	c.buf = append(c.buf, "), 'null')"...)
}

// writeJIsInteger is SQLite's test of whether a JSON number literal is an
// INTEGER: digits only, and inside int64 (a larger one is a REAL).
func (c *compileCtx) writeJIsInteger(e jsonExpr) {
	c.writeJText(e)
	c.buf = append(c.buf, " ~ '^-?[0-9]+$' AND CAST("...)
	c.writeJText(e)
	c.buf = append(c.buf, " AS numeric) BETWEEN -9223372036854775808 AND 9223372036854775807"...)
}

func (c *compileCtx) writeJBoolean(e jsonExpr, one, zero string) {
	c.buf = append(c.buf, "(CASE WHEN CAST("...)
	c.writeJ(e)
	c.buf = append(c.buf, " AS text) = 'true' THEN "...)
	c.buf = append(c.buf, one...)
	c.buf = append(c.buf, " ELSE "...)
	c.buf = append(c.buf, zero...)
	c.buf = append(c.buf, " END)"...)
}

// writeSQLiteText is SQLite's text of the value, as LIKE reads it: NULL
// stays NULL, a boolean is '1' or '0', an integer its digits (`-0` is '0'),
// everything else its JSON text (for a string, the string).
func (c *compileCtx) writeSQLiteText(e jsonExpr) {
	c.buf = append(c.buf, "(CASE "...)
	c.writeJClass(e)
	c.buf = append(c.buf, " WHEN 'null' THEN NULL WHEN 'boolean' THEN "...)
	c.writeJBoolean(e, "'1'", "'0'")
	c.buf = append(c.buf, " WHEN 'number' THEN (CASE WHEN "...)
	c.writeJIsInteger(e)
	c.buf = append(c.buf, " THEN CAST(CAST("...)
	c.writeJText(e)
	c.buf = append(c.buf, " AS numeric) AS text) ELSE "...)
	c.writeJText(e)
	c.buf = append(c.buf, " END) ELSE "...)
	c.writeJText(e)
	c.buf = append(c.buf, " END)"...)
}

// writeLike is `<text> [NOT] LIKE pattern` with SQLite's answer.
func (c *compileCtx) writeLike(e jsonExpr, pattern any, not bool) {
	_ = c.scoped(e, func(j jsonExpr) error {
		c.buf = append(c.buf, '(')
		c.writeSQLiteText(j)
		c.buf = append(c.buf, ") COLLATE \"C\""...)
		if not {
			c.buf = append(c.buf, " NOT"...)
		}
		c.buf = append(c.buf, " ILIKE CAST("...)
		c.bind(pattern)
		c.buf = append(c.buf, " AS text) ESCAPE ''"...)
		return nil
	})
}

// rhsClass is a bound value's SQLite storage class once D1 has bound it.
type rhsClass uint8

const (
	rhsNull rhsClass = iota
	rhsNum
	rhsText
	rhsBlob
)

// classify reads a value as D1 binds it: every number a REAL (D-0243), a
// non-finite one NULL (D1's JSON.stringify), a boolean 1.0 or 0.0, a string
// TEXT, bytes a BLOB.
func classify(v any) (rhsClass, float64, string, error) {
	switch x := v.(type) {
	case nil, undefinedT:
		return rhsNull, 0, "", nil
	case string:
		return rhsText, 0, x, nil
	case bool:
		if x {
			return rhsNum, 1, "", nil
		}
		return rhsNum, 0, "", nil
	case []byte:
		return rhsBlob, 0, "", nil
	case time.Time:
		return rhsText, 0, formatDate(x), nil
	}
	conv, err := Value(v)
	if err != nil {
		return 0, 0, "", err
	}
	var f float64
	switch x := conv.(type) {
	case nil:
		return rhsNull, 0, "", nil
	case string:
		return rhsText, 0, x, nil
	case bool:
		return classify(x)
	case int:
		f = float64(x)
	case int8:
		f = float64(x)
	case int16:
		f = float64(x)
	case int32:
		f = float64(x)
	case int64:
		f = float64(x)
	case uint:
		f = float64(x)
	case uint8:
		f = float64(x)
	case uint16:
		f = float64(x)
	case uint32:
		f = float64(x)
	case uint64:
		f = float64(x)
	case float32:
		f = float64(x)
	case float64:
		f = x
	default:
		return 0, 0, "", fmt.Errorf("Unsupported value type: %T", v)
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return rhsNull, 0, "", nil
	}
	return rhsNum, f, "", nil
}

// exactDecimal is the exact decimal value of a double. A double is a
// dyadic rational, so its expansion is finite (at most 1074 fractional
// digits); comparing an INTEGER with it in `numeric` is SQLite's exact
// integer-to-real comparison (sqlite3IntFloatCompare), which a float8
// comparison is not past 2^53.
func exactDecimal(f float64) string {
	s := strconv.FormatFloat(f, 'f', 1074, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// crossClass is the answer of a comparison between two storage classes that
// differ, where the row's value sorts below the bound one when rowLess.
func crossClass(op string, rowLess bool) string {
	var b bool
	switch op {
	case "=":
		b = false
	case "<>":
		b = true
	case "<", "<=":
		b = rowLess
	default:
		b = !rowLess
	}
	if b {
		return "true"
	}
	return "false"
}

var errSQLiteJSONOp = errors.New("sqlb: unknown comparison")

func validCompareOp(op string) bool {
	switch op {
	case "=", "<>", "<", "<=", ">", ">=":
		return true
	}
	return false
}

// writeCompare is `<json value> op value`, with SQLite's answer, as one
// boolean expression (NULL where SQLite's comparison is NULL).
func (c *compileCtx) writeCompare(e jsonExpr, op string, value any) error {
	if !validCompareOp(op) {
		return errSQLiteJSONOp
	}
	if _, _, _, err := classify(value); err != nil {
		return err
	}
	return c.scoped(e, func(j jsonExpr) error { return c.writeCompareBody(j, op, value) })
}

func (c *compileCtx) writeCompareBody(e jsonExpr, op string, value any) error {
	if !validCompareOp(op) {
		return errSQLiteJSONOp
	}
	cls, num, text, err := classify(value)
	if err != nil {
		return err
	}
	switch cls {
	case rhsNull:
		c.buf = append(c.buf, "CAST(NULL AS boolean)"...)
		return nil
	case rhsBlob:
		c.buf = append(c.buf, "(CASE "...)
		c.writeJClass(e)
		c.buf = append(c.buf, " WHEN 'null' THEN NULL ELSE "...)
		c.buf = append(c.buf, crossClass(op, true)...)
		c.buf = append(c.buf, " END)"...)
		return nil
	}

	c.buf = append(c.buf, "(CASE WHEN "...)
	c.writeJClass(e)
	c.buf = append(c.buf, " = 'null' THEN NULL WHEN json_typeof("...)
	c.writeJ(e)
	c.buf = append(c.buf, ") IN ('number', 'boolean') THEN "...)
	if cls == rhsText {
		// A number sorts below every text value.
		c.buf = append(c.buf, crossClass(op, true)...)
		c.buf = append(c.buf, " ELSE "...)
		c.writeJText(e)
		c.buf = append(c.buf, " COLLATE \"C\" "...)
		c.buf = append(c.buf, op...)
		c.buf = append(c.buf, " CAST("...)
		c.bind(text)
		c.buf = append(c.buf, " AS text) END)"...)
		return nil
	}

	// A number: true and false are the integers 1 and 0; an INTEGER literal
	// compares exactly; a REAL one as the double SQLite parses it to.
	exact := exactDecimal(num)
	c.buf = append(c.buf, "(CASE WHEN json_typeof("...)
	c.writeJ(e)
	c.buf = append(c.buf, ") = 'boolean' THEN "...)
	c.writeJBoolean(e, "1", "0")
	c.buf = append(c.buf, ' ')
	c.buf = append(c.buf, op...)
	c.buf = append(c.buf, " CAST(CAST("...)
	c.bind(exact)
	c.buf = append(c.buf, " AS text) AS numeric) WHEN "...)
	c.writeJIsInteger(e)
	c.buf = append(c.buf, " THEN CAST("...)
	c.writeJText(e)
	c.buf = append(c.buf, " AS numeric) "...)
	c.buf = append(c.buf, op...)
	c.buf = append(c.buf, " CAST(CAST("...)
	c.bind(exact)
	c.buf = append(c.buf, " AS text) AS numeric) ELSE CAST("...)
	c.writeJText(e)
	c.buf = append(c.buf, " AS double precision) "...)
	c.buf = append(c.buf, op...)
	c.buf = append(c.buf, " CAST("...)
	c.bind(num)
	// Every other class (string, object, array) is text, above any number.
	c.buf = append(c.buf, " AS double precision) END) ELSE "...)
	c.buf = append(c.buf, crossClass(op, false)...)
	c.buf = append(c.buf, " END)"...)
	return nil
}

// writeIsNull is `json_extract(...) IS [NOT] NULL`. One reference to the
// value, so it needs no scope.
func (c *compileCtx) writeIsNull(e jsonExpr, not bool) {
	c.buf = append(c.buf, '(')
	c.writeJClass(e)
	if not {
		c.buf = append(c.buf, " <> 'null')"...)
	} else {
		c.buf = append(c.buf, " = 'null')"...)
	}
}

// writeIn is `json_extract(...) [NOT] IN (v, …)`, as the OR of the
// equalities. Three-valued logic carries SQLite's NULL answers over: a NULL
// in the list makes a non-match NULL, and NOT of NULL is NULL.
func (c *compileCtx) writeIn(e jsonExpr, items []any, not bool) error {
	for _, item := range items {
		if _, _, _, err := classify(item); err != nil {
			return err
		}
	}
	return c.scoped(e, func(j jsonExpr) error {
		if not {
			c.buf = append(c.buf, "NOT "...)
		}
		c.buf = append(c.buf, '(')
		for i, item := range items {
			if i > 0 {
				c.buf = append(c.buf, " OR "...)
			}
			if err := c.writeCompareBody(j, "=", item); err != nil {
				return err
			}
		}
		c.buf = append(c.buf, ')')
		return nil
	})
}

// writeOrderBy is ORDER BY json_extract(...) in SQLite's order: NULL, then
// numbers by value, then text byte-wise. Three keys, one direction.
func (c *compileCtx) writeOrderBy(e jsonExpr, desc bool) {
	dir := " ASC"
	if desc {
		dir = " DESC"
	}
	_ = c.scoped(e, func(j jsonExpr) error {
		c.buf = append(c.buf, "CASE WHEN "...)
		c.writeJClass(j)
		c.buf = append(c.buf, " = 'null' THEN 0 WHEN json_typeof("...)
		c.writeJ(j)
		c.buf = append(c.buf, ") IN ('number', 'boolean') THEN 1 ELSE 2 END"...)
		return nil
	})
	c.buf = append(c.buf, dir...)
	c.buf = append(c.buf, ", "...)
	_ = c.scoped(e, func(j jsonExpr) error {
		c.buf = append(c.buf, "CASE json_typeof("...)
		c.writeJ(j)
		c.buf = append(c.buf, ") WHEN 'number' THEN CAST("...)
		c.writeJText(j)
		c.buf = append(c.buf, " AS numeric) WHEN 'boolean' THEN "...)
		c.writeJBoolean(j, "1", "0")
		c.buf = append(c.buf, " END"...)
		return nil
	})
	c.buf = append(c.buf, dir...)
	c.buf = append(c.buf, ", "...)
	_ = c.scoped(e, func(j jsonExpr) error {
		c.buf = append(c.buf, "CASE WHEN json_typeof("...)
		c.writeJ(j)
		c.buf = append(c.buf, ") IN ('string', 'object', 'array') THEN "...)
		c.writeJText(j)
		c.buf = append(c.buf, " END"...)
		return nil
	})
	c.buf = append(c.buf, " COLLATE \"C\""...)
	c.buf = append(c.buf, dir...)
}

// sqliteJSONField is a filter's dotted field as a [jsonExpr].
func (c *compileCtx) sqliteJSONField(alias, column, path string) (jsonExpr, error) {
	steps, err := parseSQLitePath(path)
	if err != nil {
		return jsonExpr{}, err
	}
	return jsonExpr{col: appendQualified(nil, alias, column, c.dialect.quote), steps: steps}, nil
}

// --- the exported building blocks ------------------------------------------------

// helperCtx is a compile context producing a composable `?` Fragment in d's
// spelling.
func (d Dialect) helperCtx() *compileCtx {
	q := d.resolve()
	q.placeholders = placeholderQuestion
	return &compileCtx{dialect: q, buf: make([]byte, 0, 256)}
}

func (c *compileCtx) fragment() Fragment {
	return Fragment{text: string(c.buf), values: c.values}
}

// columnText is a column reference fragment as text, refusing one that binds
// values: the expression is written several times, and its values would have
// to be repeated with it.
func columnText(column Fragment) ([]byte, error) {
	if column.err != nil {
		return nil, column.err
	}
	if n, _ := scanSQL(column.text); column.text == "" || len(column.values) > 0 || n > 0 {
		return nil, errors.New("sqlb: the column must be an identifier fragment with no bound values")
	}
	return []byte(column.text), nil
}

func (d Dialect) needs(o Option) error {
	d = d.resolve()
	switch {
	case o == SQLiteJSONPath && d.sqliteJSON, o == SQLiteLike && d.sqliteLike:
		return nil
	}
	return fmt.Errorf("sqlb: the %s dialect was not given the option this needs", d.name)
}

// CompareJSONPath renders `json_extract(column, path) <op> value` as SQLite
// answers it, for a dialect given [SQLiteJSONPath]. column is an identifier
// fragment ([Ident]); path is SQLite's (`$.a.b`). op is one of = <> < <= > >=,
// `like` and `not like` (with SQLite's LIKE, as [SQLiteLike]), `is null` and
// `is not null` (value ignored), or `in` and `not in` (value a []any).
//
// The result composes inside [Q], with `?` markers.
func (d Dialect) CompareJSONPath(column Fragment, path, op string, value any) (Fragment, error) {
	if err := d.needs(SQLiteJSONPath); err != nil {
		return Fragment{}, err
	}
	col, err := columnText(column)
	if err != nil {
		return Fragment{}, err
	}
	steps, err := parseSQLitePath(path)
	if err != nil {
		return Fragment{}, err
	}
	e := jsonExpr{col: col, steps: steps}
	c := d.helperCtx()
	switch op {
	case "is null", "is not null":
		c.writeIsNull(e, op == "is not null")
	case "like", "not like":
		c.writeLike(e, value, op == "not like")
	case "in", "not in":
		items, ok := value.([]any)
		if !ok {
			return Fragment{}, errors.New("sqlb: in and not in take a []any")
		}
		if err := c.writeIn(e, items, op == "not in"); err != nil {
			return Fragment{}, err
		}
	default:
		if err := c.writeCompare(e, op, value); err != nil {
			return Fragment{}, err
		}
	}
	return c.fragment(), nil
}

// OrderByJSONPath renders `json_extract(column, path) ASC|DESC` as SQLite
// orders it — NULL, then numbers, then text — for a dialect given
// [SQLiteJSONPath]. It is three ORDER BY keys, comma-separated.
func (d Dialect) OrderByJSONPath(column Fragment, path string, desc bool) (Fragment, error) {
	if err := d.needs(SQLiteJSONPath); err != nil {
		return Fragment{}, err
	}
	col, err := columnText(column)
	if err != nil {
		return Fragment{}, err
	}
	steps, err := parseSQLitePath(path)
	if err != nil {
		return Fragment{}, err
	}
	c := d.helperCtx()
	c.writeOrderBy(jsonExpr{col: col, steps: steps}, desc)
	return c.fragment(), nil
}

// LikeSQLite renders `column [NOT] LIKE pattern` as SQLite answers it, for a
// dialect given [SQLiteLike]: whatever the column's type, its SQLite text is
// matched case-insensitively for ASCII, with no escape character.
func (d Dialect) LikeSQLite(column Fragment, pattern any, not bool) (Fragment, error) {
	if err := d.needs(SQLiteLike); err != nil {
		return Fragment{}, err
	}
	col, err := columnText(column)
	if err != nil {
		return Fragment{}, err
	}
	c := d.helperCtx()
	c.writeLike(jsonExpr{col: col, toJSON: true}, pattern, not)
	return c.fragment(), nil
}
