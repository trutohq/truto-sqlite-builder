package sqlb

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// --- SQLite's numeric affinity ---------------------------------------------------

// sqliteNumeric is the text SQLite's NUMERIC affinity converts: optional
// whitespace, a sign, decimal digits with an optional point, an optional
// exponent — `+3`, `1.`, `.5e1`, ` 3 ` — and nothing else (no hex, no `inf`,
// no `1e`). Measured on modernc (gotruto's FuzzSQLiteNumberMatchesAffinity
// holds it to SQLite on arbitrary input).
var sqliteNumeric = regexp.MustCompile(`^[ \t\n\v\f\r]*[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?[ \t\n\v\f\r]*$`)

var sqliteInteger = regexp.MustCompile(`^[ \t\n\v\f\r]*[+-]?[0-9]+[ \t\n\v\f\r]*$`)

// SQLiteNumber reads text the way SQLite's NUMERIC and INTEGER affinities
// do: an integer literal within int64 is an INTEGER (isInt, i), any other
// numeric text a REAL (f), and anything else stays text (ok false). An
// overflowing literal is ±Inf, as in SQLite ('1e400').
//
// SQLite reads the text only up to a NUL byte, so `1\x00abc` is 1.
func SQLiteNumber(s string) (f float64, i int64, isInt, ok bool) {
	if nul := strings.IndexByte(s, 0); nul >= 0 {
		s = s[:nul]
	}
	if !sqliteNumeric.MatchString(s) {
		return 0, 0, false, false
	}
	t := strings.Trim(s, " \t\n\v\f\r")
	if sqliteInteger.MatchString(t) {
		if n, err := strconv.ParseInt(strings.TrimPrefix(t, "+"), 10, 64); err == nil {
			return float64(n), n, true, true
		}
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, 0, false, false
	}
	return f, 0, false, true
}

// RealToInt is SQLite's exact REAL-to-INTEGER: a REAL holding an integer
// strictly inside int64's range, and nothing else. -2^63 itself stays a REAL
// in SQLite (measured: `-9223372036854775809` is stored REAL).
func RealToInt(f float64) (int64, bool) {
	if f != math.Trunc(f) || math.IsNaN(f) || f <= -(1<<63) || f >= 1<<63 {
		return 0, false
	}
	return int64(f), true
}

// --- column kinds -------------------------------------------------------------------

// ColumnKind is what a Postgres column holds, as SQLite's comparison rules
// need to know it: the affinity SQLite's column had, and so how a value
// compared with it is converted first.
type ColumnKind uint8

const (
	// ColumnUnknown is not a column at all. SQLite reads a double-quoted
	// name it cannot resolve as a string LITERAL, so `"nosuchcol" = 'x'` is
	// false and `"nosuchcol" = 'nosuchcol'` true — never an error.
	ColumnUnknown ColumnKind = iota
	// ColumnText is TEXT affinity: text, ids, JSON documents.
	ColumnText
	// ColumnInteger is INTEGER affinity, a bigint.
	ColumnInteger
	// ColumnReal is REAL affinity, a double precision.
	ColumnReal
	// ColumnBoolean is SQLite's 1 and 0 in a NUMERIC column, a boolean.
	ColumnBoolean
	// ColumnOther is any column whose comparisons are left as written: the
	// date-time and date columns, whose values the driver parses.
	ColumnOther
)

// affinityOpt is [SQLiteAffinity]'s option. Held by pointer in the Dialect
// so a Dialect stays comparable.
type affinityOpt struct {
	kind func(column string) ColumnKind
}

func (a *affinityOpt) apply(d *Dialect) error {
	if a == nil || a.kind == nil {
		return errors.New("sqlb: SQLiteAffinity needs a column lookup")
	}
	d.affinity = a
	return nil
}

// SQLiteAffinity makes a plain field's comparisons answer as SQLite's do
// against its column's affinity, for a dialect also given [SQLiteLike] or
// [SQLiteJSONPath] or neither. kind answers for a field name as the filter
// writes it (an alias block's fields are looked up by their bare name);
// [ColumnUnknown] makes it SQLite's string literal.
//
// What changes, on Postgres:
//
//   - numeric text against a numeric column is the number (NUMERIC affinity),
//     compared exactly, even when it is fractional against a bigint;
//   - other text against a numeric or boolean column is above every number,
//     so `count = 'abc'` is false and `count < 'abc'` true for every row
//     with a count — where a typed parameter would refuse the statement;
//   - a number against a text column is passed through for the driver to
//     render as text (SQLite's own rendering, '5.0', is not reproduced);
//   - text containing U+0000, which Postgres text cannot hold, is compared
//     byte-wise against the column's UTF-8, as SQLite compares text;
//   - an unknown name is the literal, compared in Go.
func SQLiteAffinity(kind func(column string) ColumnKind) Option {
	return &affinityOpt{kind: kind}
}

// --- comparisons against a column ---------------------------------------------------

// sqliteCompareValues answers `a op b` for two constants under SQLite's
// rules: NULL is NULL; across storage classes NULL < numbers < text < blob;
// numbers by value, text and blobs byte-wise.
func sqliteCompareValues(acls rhsClass, anum float64, atext string, bcls rhsClass, bnum float64, btext string, op string) (bool, bool) {
	if acls == rhsNull || bcls == rhsNull {
		return false, false
	}
	var c int
	switch {
	case acls != bcls:
		if acls < bcls {
			c = -1
		} else {
			c = 1
		}
	case acls == rhsNum:
		switch {
		case anum < bnum:
			c = -1
		case anum > bnum:
			c = 1
		}
	default:
		c = strings.Compare(atext, btext)
	}
	switch op {
	case "=":
		return c == 0, true
	case "<>":
		return c != 0, true
	case "<":
		return c < 0, true
	case "<=":
		return c <= 0, true
	case ">":
		return c > 0, true
	}
	return c >= 0, true
}

// writeBool writes a constant answer; a NULL one as a NULL boolean.
func (c *compileCtx) writeBool(b, known bool) {
	switch {
	case !known:
		c.buf = append(c.buf, "CAST(NULL AS boolean)"...)
	case b:
		c.buf = append(c.buf, "true"...)
	default:
		c.buf = append(c.buf, "false"...)
	}
}

// writeCrossClass is the answer for a non-NULL column value of a different
// storage class than the bound one: constant, and NULL where the column is.
func (c *compileCtx) writeCrossClass(col []byte, op string, rowLess bool) {
	c.buf = append(c.buf, "(CASE WHEN "...)
	c.buf = append(c.buf, col...)
	c.buf = append(c.buf, " IS NULL THEN NULL ELSE "...)
	c.buf = append(c.buf, crossClass(op, rowLess)...)
	c.buf = append(c.buf, " END)"...)
}

// writeColumnCompare writes `col op value` with SQLite's answer for a column
// of the given kind.
func (c *compileCtx) writeColumnCompare(col []byte, name string, kind ColumnKind, op string, value any) error {
	if !validCompareOp(op) {
		return errSQLiteJSONOp
	}
	cls, num, text, err := classify(value)
	if err != nil {
		return err
	}
	if kind == ColumnUnknown {
		b, known := sqliteCompareValues(rhsText, 0, name, cls, num, text, op)
		c.writeBool(b, known)
		return nil
	}
	if cls == rhsNull {
		c.writeBool(false, false)
		return nil
	}
	if cls == rhsBlob {
		// Every class but blob sorts below a blob.
		c.writeCrossClass(col, op, true)
		return nil
	}

	switch kind {
	case ColumnText:
		if cls == rhsText && strings.IndexByte(text, 0) >= 0 {
			// Postgres text cannot hold U+0000, so no stored value contains
			// it; SQLite compares text with memcmp, which is bytea's order.
			c.buf = append(c.buf, "convert_to("...)
			c.buf = append(c.buf, col...)
			c.buf = append(c.buf, ", 'UTF8') "...)
			c.buf = append(c.buf, op...)
			c.buf = append(c.buf, " CAST("...)
			c.bind([]byte(text))
			c.buf = append(c.buf, " AS bytea)"...)
			return nil
		}
		c.buf = append(c.buf, col...)
		c.buf = append(c.buf, ' ')
		c.buf = append(c.buf, op...)
		c.buf = append(c.buf, " CAST("...)
		c.bind(value)
		c.buf = append(c.buf, " AS text)"...)
		return nil

	case ColumnInteger, ColumnReal, ColumnBoolean:
		if cls == rhsText {
			f, i, isInt, ok := SQLiteNumber(text)
			if !ok {
				// Text that is not a number stays text, above every number.
				c.writeCrossClass(col, op, true)
				return nil
			}
			if isInt {
				return c.writeNumericCompare(col, kind, op, float64(i), i, true)
			}
			num = f
		}
		i, isInt := RealToInt(num)
		return c.writeNumericCompare(col, kind, op, num, i, isInt)
	}

	// ColumnOther: as written; the driver decides what the value means.
	c.buf = append(c.buf, col...)
	c.buf = append(c.buf, ' ')
	c.buf = append(c.buf, op...)
	c.buf = append(c.buf, ' ')
	c.bind(value)
	return nil
}

// writeNumericCompare compares a numeric column with a number, exactly as
// SQLite does: an INTEGER with a REAL by exact value, never through a lossy
// cast. isInt reports that num is the integer i.
func (c *compileCtx) writeNumericCompare(col []byte, kind ColumnKind, op string, num float64, i int64, isInt bool) error {
	if math.IsInf(num, 0) {
		// Every finite value is below +Inf and above -Inf.
		c.writeCrossClass(col, op, num > 0)
		return nil
	}
	switch {
	case kind == ColumnReal:
		c.buf = append(c.buf, col...)
		c.buf = append(c.buf, ' ')
		c.buf = append(c.buf, op...)
		c.buf = append(c.buf, " CAST("...)
		c.bind(num)
		c.buf = append(c.buf, " AS double precision)"...)
	case kind == ColumnBoolean && isInt && (i == 0 || i == 1):
		c.buf = append(c.buf, col...)
		c.buf = append(c.buf, ' ')
		c.buf = append(c.buf, op...)
		c.buf = append(c.buf, " CAST("...)
		c.bind(i == 1)
		c.buf = append(c.buf, " AS boolean)"...)
	case kind == ColumnBoolean:
		// SQLite holds 1 and 0; anything else compares with those.
		c.buf = append(c.buf, "CAST("...)
		c.buf = append(c.buf, col...)
		c.buf = append(c.buf, " AS integer) "...)
		c.buf = append(c.buf, op...)
		c.buf = append(c.buf, " CAST(CAST("...)
		c.bind(exactDecimal(num))
		c.buf = append(c.buf, " AS text) AS numeric)"...)
	case isInt:
		c.buf = append(c.buf, col...)
		c.buf = append(c.buf, ' ')
		c.buf = append(c.buf, op...)
		c.buf = append(c.buf, " CAST("...)
		c.bind(i)
		c.buf = append(c.buf, " AS bigint)"...)
	default:
		// A fraction, or past int64, against a bigint: exact, in numeric.
		c.buf = append(c.buf, "CAST("...)
		c.buf = append(c.buf, col...)
		c.buf = append(c.buf, " AS numeric) "...)
		c.buf = append(c.buf, op...)
		c.buf = append(c.buf, " CAST(CAST("...)
		c.bind(exactDecimal(num))
		c.buf = append(c.buf, " AS text) AS numeric)"...)
	}
	return nil
}

// writeColumnIn is `col [NOT] IN (…)`: the OR of the equalities, which is
// what SQLite's IN means, NULLs included (a NULL in the list makes a
// non-match NULL, and NOT of NULL is NULL). An empty list is SQLite's: IN
// is false and NOT IN true, even for a NULL column.
func (c *compileCtx) writeColumnIn(col []byte, name string, kind ColumnKind, items []any, not bool) error {
	if len(items) == 0 {
		c.writeBool(not, true)
		return nil
	}
	if not {
		c.buf = append(c.buf, "NOT "...)
	}
	c.buf = append(c.buf, '(')
	for i, item := range items {
		if i > 0 {
			c.buf = append(c.buf, " OR "...)
		}
		if err := c.writeColumnCompare(col, name, kind, "=", item); err != nil {
			return err
		}
	}
	c.buf = append(c.buf, ')')
	return nil
}

// CompareColumn renders `column <op> value` as SQLite answers it against a
// column of the given kind, for a dialect given [SQLiteAffinity] (the kind is
// the caller's here). name is the column's bare name, which is the literal an
// unknown column is. op is one of = <> < <= > >=, or `in` and `not in` with
// value a []any (an empty list is SQLite's answer, not an error). Values are
// read as D1 binds them ([SQLiteJSONPath] says how).
func (d Dialect) CompareColumn(column Fragment, name string, kind ColumnKind, op string, value any) (Fragment, error) {
	if d.resolve().affinity == nil {
		return Fragment{}, fmt.Errorf("sqlb: the %s dialect was not given SQLiteAffinity", d.resolve().name)
	}
	col, err := columnText(column)
	if err != nil {
		return Fragment{}, err
	}
	c := d.helperCtx()
	switch op {
	case "in", "not in":
		items, ok := value.([]any)
		if !ok {
			return Fragment{}, errors.New("sqlb: in and not in take a []any")
		}
		if err := c.writeColumnIn(col, name, kind, items, op == "not in"); err != nil {
			return Fragment{}, err
		}
	default:
		if err := c.writeColumnCompare(col, name, kind, op, value); err != nil {
			return Fragment{}, err
		}
	}
	return c.fragment(), nil
}

// LikeLiteral is SQLite's `'name' [NOT] LIKE pattern` for an unknown column,
// whose name SQLite reads as a string literal.
func (d Dialect) LikeLiteral(name string, pattern any, not bool) (Fragment, error) {
	if err := d.needs(SQLiteLike); err != nil {
		return Fragment{}, err
	}
	c := d.helperCtx()
	c.buf = append(c.buf, "(CAST("...)
	c.bind(name)
	c.buf = append(c.buf, " AS text) COLLATE \"C\""...)
	if not {
		c.buf = append(c.buf, " NOT"...)
	}
	c.buf = append(c.buf, " ILIKE CAST("...)
	c.bind(pattern)
	c.buf = append(c.buf, " AS text) ESCAPE '')"...)
	return c.fragment(), nil
}

// colTarget is a plain field under [SQLiteAffinity]: its rendered reference,
// its bare name, and what it holds.
type colTarget struct {
	col  []byte
	name string
	kind ColumnKind
}

func (c *compileCtx) columnTarget(alias, column string) colTarget {
	return colTarget{
		col:  appendQualified(nil, alias, column, c.dialect.quote),
		name: column,
		kind: c.dialect.affinity.kind(column),
	}
}

// writeIsNull is `col IS [NOT] NULL`; a literal is never NULL.
func (t colTarget) writeIsNull(c *compileCtx, not bool) {
	if t.kind == ColumnUnknown {
		c.writeBool(not, true)
		return
	}
	c.buf = append(c.buf, t.col...)
	if not {
		c.buf = append(c.buf, " IS NOT NULL"...)
	} else {
		c.buf = append(c.buf, " IS NULL"...)
	}
}
