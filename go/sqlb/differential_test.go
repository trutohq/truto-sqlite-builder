// Differential testing against the TypeScript reference.
//
// goenvoy's recorded lesson from the same programme is that every real
// divergence it found came from EXECUTING both engines, and none from reading
// the TypeScript. So this generates a large deterministic corpus, runs it
// through bun and through this package, and diffs the results — text, values
// and error messages.
//
// The runner itself is github.com/trutohq/differential — Truto's H-1. Case
// generation, the bun subprocess, the TZ=UTC pin, result diffing, artifacts,
// the reproduction hint and skip-when-bun-absent all live there, because this
// package and jsorder had grown two copies of the same loop and gotruto was
// about to make a third (D-0013). What stays here is the part that is actually
// about SQL: the corpus, and what counts as agreement.
//
// It skips when bun or the reference checkout is unavailable, so `go test`
// still works standalone. TRUTO_DIFFERENTIAL=required turns that skip into a
// failure (D-0079). Run it explicitly with:
//
//	go test -run TestDifferential -v ./...
package sqlb

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/trutohq/differential"
)

// diffCase is one generated case, in a shape both engines can consume.
type diffCase struct {
	Kind string `json:"kind"` // "filter" or "sql"

	// Filter cases.
	Filter json.RawMessage `json:"filter,omitempty"`

	// Query cases: literal chunks with one argument between each pair.
	Parts []string  `json:"parts"`
	Args  []diffArg `json:"args"`
}

// diffArg is a tagged interpolation argument. The tag says which helper, if
// any, builds it, so both engines construct the same thing.
type diffArg struct {
	T string `json:"t"` // str|num|bool|null|undefined|ident|idents|raw|in|date|blob
	V any    `json:"v,omitempty"`
}

type diffResult struct {
	OK     bool   `json:"ok"`
	Text   string `json:"text,omitempty"`
	Values []any  `json:"values,omitempty"`
	Error  string `json:"error,omitempty"`

	// VTypes is what SQLite would STORE for each bound value, one tag per
	// entry of Values. See valueTag, and run.ts's DiffResult.vtypes.
	VTypes []string `json:"vtypes,omitempty"`
}

// valueTag names what SQLite would store for one bound value.
//
// It exists because the JSON wire format erases Go type identity, and
// canonValues therefore could not tell an INTEGER from a REAL: json.Marshal
// renders int64(1) and float64(1) both as `1`, and JSON.parse("1.0") is 1 on
// the reference side. Changing sqlValue's `case int` to return `float64(x)` —
// which binds EVERY integer argument as a SQLite REAL — left 25,000 cases green
// with identical bucket counts. That is the data-corruption class, not the
// panic class: typeof() and quote() differ, and a value read back
// re-serialises as 1.0.
//
// The reference computes the same tag from what its driver would bind
// (run.ts:valueTag), which is why `int` here means the Go integer types rather
// than "a float that happens to be integral": an integral float64 handed to a
// driver binds REAL, and that is the fault being caught. jsNumber
// (jsonfilter.go) is the code responsible for producing an int64 where
// JavaScript has an integral number.
func valueTag(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "text"
	case bool:
		return "bool"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "int"
	case float32, float64:
		return "real"
	case []byte:
		return "blob"
	}
	return fmt.Sprintf("%T", v)
}

func valueTags(vs []any) []string {
	if len(vs) == 0 {
		return nil
	}
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = valueTag(v)
	}
	return out
}

func TestDifferential(t *testing.T) {
	// Corpus invariants, checked in a Cleanup so they are still reported when
	// the diff has already failed the test. They are deliberately NOT modelled
	// as mismatches: a mismatch is something a future divergence oracle may
	// legitimately suppress, and "the generator produced a case the two engines
	// are not being asked to agree on" must never be suppressible.
	var invariants []string
	t.Cleanup(func() {
		if len(invariants) > 0 {
			t.Errorf("%d corpus invariant violations; first: %s",
				len(invariants), invariants[0])
		}
	})

	cov := &coverage{
		wantKinds:  map[string]int{"filter": 5, "sql": 5}, // each at least a fifth
		wantLabels: map[string]int{"output": 10, "error": 10},
		minShapes:  20,
	}
	t.Cleanup(func() { t.Log(cov.summary()) })

	differential.CheckCorpus(t, differential.CorpusSpec[diffCase, diffResult]{
		Name: "sqlb",
		Reference: differential.Reference{
			Script: "../differential/run.ts",
			Dir:    "../..",
			// The reference is TypeScript with extensionless relative
			// imports, which only bun resolves. Taking the default runtime
			// list would fail confusingly under node rather than skipping.
			Runtimes:     []string{"bun"},
			RequireFiles: []string{"src/sql.ts"},
		},
		// Both PCG words are pinned. Changing either changes the corpus, and
		// the seeds in README.md — 150,000 cases across six of them with zero
		// mismatches — are claims about this exact corpus.
		Seed:      0x5150,
		Stream:    0x1979,
		Cases:     4000,
		EnvPrefix: "SQLB_DIFF",
		Generate:  generateCases,
		Kind:      func(c diffCase) string { return c.Kind },
		Eval: func(c diffCase) diffResult {
			r, invariant := runCase(c)
			if invariant != "" {
				invariants = append(invariants, invariant)
			}
			return r
		},
		Compare:    cov.compare,
		Classifier: cov,
	})
}

// TestDifferentialValueTypes is a SECOND corpus, for the argument kinds the
// main one cannot reach.
//
// It is separate rather than folded into genArg because the main corpus's bytes
// are pinned per seed (TestCorpusBytesAreUnchanged) and the six recorded
// 150,000-case runs are claims about exactly those cases. Widening genArg would
// silently retire that history.
//
// What it adds, all of it previously untested by any differential:
//
//   - `date`. formatDate's two load-bearing quirks — the year is NOT
//     zero-padded, and every other field is — were "confirmed against the
//     reference" by hand and never executed against it. Years 999 and 1 are in
//     the corpus for exactly that.
//   - The TZ=UTC pin (H-18) gets exercised by a corpus rather than only by the
//     runner's own unit test. formatDate reads local-time getters on both sides,
//     so an unpinned TZ diverges here and nowhere else.
//   - `blob`, `rawblob` and the sql.blob() argument check: three throw sites
//     the main corpus cannot reach, plus the Uint8Array-as-base64 wire rule.
//   - `Unsupported value type:`, which is the ONE message sameError is
//     deliberately loose about. That exemption never fired in 150,000 cases, so
//     the single place the comparison is loosened was itself untested.
func TestDifferentialValueTypes(t *testing.T) {
	cov := &coverage{
		wantKinds: map[string]int{"sql": 1},
		// Both outcomes, because half these tags exist to reach a throw.
		wantLabels: map[string]int{"output": 5, "error": 5},
		minShapes:  3,
		// The loose comparison must actually be exercised here, since it is
		// exercised nowhere else.
		minLooseErrors: 1,
	}
	t.Cleanup(func() { t.Log(cov.summary()) })

	differential.CheckCorpus(t, differential.CorpusSpec[diffCase, diffResult]{
		Name: "sqlb-value-types",
		Reference: differential.Reference{
			Script:       "../differential/run.ts",
			Dir:          "../..",
			Runtimes:     []string{"bun"},
			RequireFiles: []string{"src/sql.ts"},
		},
		Seed:       0x1D0C,
		Stream:     0x1979,
		Cases:      2000,
		EnvPrefix:  "SQLB_VTYPE_DIFF",
		Generate:   generateValueTypeCases,
		Kind:       func(c diffCase) string { return c.Kind },
		Eval:       func(c diffCase) diffResult { r, _ := runCase(c); return r },
		Compare:    cov.compare,
		Classifier: cov,
	})
}

// coverage is the H-2 seam wired up as a set of floors.
//
// A differential that agrees on everything it looks at still proves nothing
// about what it stopped looking at, and nothing was measuring that: the corpus
// reaches 24 of the reference's 47 throw sites, two documented argument tags
// were never generated, and the one deliberately-loose comparison had never
// fired. Coverage.Kinds and Coverage.Labels are the seam for saying so, and
// they had no consumer.
//
// It classifies nothing. Only an INTENDED `I-` row may suppress, sqlb has no
// such row for the differential, and a Classifier that suppressed anything here
// would be loosening the comparison rather than measuring it.
type coverage struct {
	// wantKinds and wantLabels map a name to the minimum share of the run it
	// must account for, as a divisor: 5 means "at least a fifth".
	wantKinds  map[string]int
	wantLabels map[string]int

	// minShapes is how many DISTINCT reference error messages the corpus must
	// reach. It is the only measure of error-path coverage there is; a
	// one-character change to a message the corpus never produces is invisible.
	minShapes int

	// minLooseErrors is how many times sameError's typeof exemption must fire.
	minLooseErrors int

	shapes      map[string]int
	looseErrors int
	// cases counts compare calls, so a run that SKIPPED — no bun, no reference
	// checkout — does not log a coverage line reading "0 reached (floor 11)".
	cases int
}

// summary is what the floors are measuring, so a reader can see the margin
// rather than only hearing about it when it is breached. Logged on every run
// that happened.
func (c *coverage) summary() string {
	if c.cases == 0 {
		return "coverage: the differential did not run"
	}
	return fmt.Sprintf("coverage: %d distinct reference error messages reached "+
		"(floor %d); sameError's typeof exemption fired %d times (floor %d)",
		len(c.shapes), c.minShapes, c.looseErrors, c.minLooseErrors)
}

// compare wraps compareResults to record what the run reached. It is called
// once per case, from the runner's diff loop, sequentially.
func (c *coverage) compare(got, want diffResult) differential.Diff {
	c.cases++
	if !want.OK {
		if c.shapes == nil {
			c.shapes = map[string]int{}
		}
		c.shapes[errShape(want.Error)]++
		if got.Error != want.Error && sameError(got.Error, want.Error) {
			c.looseErrors++
		}
	}
	return compareResults(got, want)
}

func (c *coverage) Classify(differential.Mismatch) differential.Class {
	return differential.Class{}
}

func (c *coverage) Observed(cv differential.Coverage) error {
	var problems []string
	for name, divisor := range c.wantKinds {
		if cv.Kinds[name]*divisor < cv.Cases {
			problems = append(problems, fmt.Sprintf(
				"only %d of %d cases are kind %q, want at least a %dth of the run",
				cv.Kinds[name], cv.Cases, name, divisor))
		}
	}
	for name, divisor := range c.wantLabels {
		if cv.Labels[name]*divisor < cv.Cases {
			problems = append(problems, fmt.Sprintf(
				"only %d of %d cases agreed with label %q, want at least a %dth of "+
					"the run — a corpus that has quietly become all-%s passes while "+
					"testing almost nothing",
				cv.Labels[name], cv.Cases, name, divisor,
				map[string]string{"output": "errors", "error": "output"}[name]))
		}
	}
	if len(c.shapes) < c.minShapes {
		problems = append(problems, fmt.Sprintf(
			"the corpus reached %d distinct reference error messages over %d cases, want "+
				"at least %d; an error path the corpus never produces is one a "+
				"one-character change can be made to silently. NOTE: a run smaller than "+
				"the default case count cannot satisfy this floor, deliberately — a "+
				"reduced *_CASES is not a gate",
			len(c.shapes), cv.Cases, c.minShapes))
	}
	if c.looseErrors < c.minLooseErrors {
		problems = append(problems, fmt.Sprintf(
			"sameError's typeof exemption fired %d times, want at least %d; the one "+
				"place the comparison is deliberately loose must be exercised, or it is "+
				"an untested loosening", c.looseErrors, c.minLooseErrors))
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("corpus coverage floor: %s", strings.Join(problems, "; "))
	}
	return nil
}

// errVariable matches the per-case parts of a reference error message: quoted
// fragments and bare numbers.
var errVariable = regexp.MustCompile(`'[^']*'|"[^"]*"|\d+`)

// errShape reduces a message to the throw site it came from, so the count means
// "how many error paths did the corpus reach" rather than "how many invalid
// identifiers did it try".
//
// Two normalisations, both needed. Quoted fragments and numbers go first —
// `Identifier part too long: 300 characters (max: 255)`. Then everything after
// the first ": " is dropped, because the reference interpolates its variable
// part there UNquoted: the eleven distinct `Invalid identifier: <name>. Must
// be …` messages are one throw site (src/sql.ts:74), and counting them as
// eleven would make the floor look satisfied by a corpus that had stopped
// reaching anything else.
func errShape(msg string) string {
	shape := errVariable.ReplaceAllString(msg, "_")
	if i := strings.Index(shape, ": "); i >= 0 {
		shape = shape[:i+1]
	}
	if len(shape) > 64 {
		shape = shape[:64]
	}
	return shape
}

// compareResults is sqlb's definition of agreement.
//
// The ORDER of the checks is load-bearing and reproduces what the hand-rolled
// loop did: at most one difference is reported per case, and the ok flag
// short-circuits the rest, because two engines that disagree about whether a
// filter compiles at all have nothing to say to each other about its text.
func compareResults(got, want diffResult) differential.Diff {
	if got.OK != want.OK {
		return differential.Diff{Label: "output", Fields: []differential.FieldDiff{{
			Field: "ok",
			Got:   fmt.Sprintf("ok=%v text=%q error=%q", got.OK, got.Text, got.Error),
			Want:  fmt.Sprintf("ok=%v error=%q", want.OK, want.Error),
		}}}
	}
	if !got.OK {
		// Both threw. The MESSAGES are compared too, not just the fact of
		// failing: roughly 59% of the corpus lands here, so treating an error
		// as an unconditional agreement would make most of the run vacuous.
		if sameError(got.Error, want.Error) {
			return differential.Diff{Label: "error"}
		}
		return differential.Diff{Label: "error", Fields: []differential.FieldDiff{{
			Field: "error", Got: got.Error, Want: want.Error,
		}}}
	}
	if got.Text != want.Text {
		return differential.Diff{Label: "output", Fields: []differential.FieldDiff{{
			Field: "text", Got: got.Text, Want: want.Text,
		}}}
	}
	if gv, wv := canonValues(got.Values), canonValues(want.Values); gv != wv {
		return differential.Diff{Label: "output", Fields: []differential.FieldDiff{{
			Field: "values", Got: gv, Want: wv,
		}}}
	}
	// The values agree as JSON; do they agree as SQLite types? JSON cannot
	// express the difference, so it travels as a separate tag per value — see
	// valueTag. Checked last because a text or value difference is the more
	// informative report when both are present.
	if gt, wt := strings.Join(got.VTypes, ","), strings.Join(want.VTypes, ","); gt != wt {
		return differential.Diff{Label: "output", Fields: []differential.FieldDiff{{
			Field: "valuetypes", Got: gt, Want: wt,
		}}}
	}
	return differential.Diff{Label: "output"}
}

// sameError compares error text, allowing for the one message shape that
// cannot be identical: the reference reports JavaScript's typeof, Go reports
// its own type name. Everything else must match verbatim.
func sameError(goErr, jsErr string) bool {
	if goErr == jsErr {
		return true
	}
	const unsupported = "Unsupported value type:"
	if strings.HasPrefix(goErr, unsupported) && strings.HasPrefix(jsErr, unsupported) {
		return true
	}
	return false
}

func canonValues(vs []any) string {
	if len(vs) == 0 {
		return "[]"
	}
	b, err := json.Marshal(vs)
	if err != nil {
		return fmt.Sprintf("<unmarshalable: %v>", err)
	}
	return string(b)
}

// runCase evaluates one case against this package. The second return is a
// corpus invariant violation rather than a mismatch — see TestDifferential.
func runCase(c diffCase) (diffResult, string) {
	switch c.Kind {
	case "filter":
		filter, err := DecodeFilter(c.Filter)
		if err != nil {
			return diffResult{Error: err.Error()}, ""
		}
		frag, err := CompileFilter(filter)
		if err != nil {
			return diffResult{Error: err.Error()}, ""
		}
		// The one case the two engines are no longer asked to agree on: since
		// D-0032 a filter with no conditions compiles to no WHERE clause here and
		// throws in the reference. The generator cannot currently produce one —
		// genFilterNode always writes at least one entry, and every entry either
		// emits a clause or errors — so rather than teach the comparison an
		// exception, which would blunt it for every other case, this fails loudly
		// if that ever stops being true. Widening the generator is fine; silently
		// comparing a deliberate divergence is not.
		if frag.IsEmpty() {
			return okResult(frag),
				fmt.Sprintf("the corpus produced a top-level empty filter (%s), which "+
					"D-0032 makes an INTENDED divergence — exclude it from the "+
					"generator or compare it separately", c.Filter)
		}
		return okResult(frag), ""
	case "sql":
		args := make([]any, len(c.Args))
		for i, a := range c.Args {
			args[i] = buildArg(a)
		}
		// Converting to sqlText is only possible inside this package, which is
		// the point of the type — the differential harness is the one caller that
		// legitimately needs a dynamic format.
		frag, err := Q(sqlText(strings.Join(c.Parts, "?")), args...)
		if err != nil {
			return diffResult{Error: err.Error()}, ""
		}
		return okResult(frag), ""
	}
	return diffResult{Error: "harness: unknown case kind " + c.Kind},
		"unknown case kind " + c.Kind
}

func okResult(frag Fragment) diffResult {
	vs := frag.Values()
	return diffResult{OK: true, Text: frag.Text(), Values: vs, VTypes: valueTags(vs)}
}

func buildArg(a diffArg) any {
	switch a.T {
	case "str", "num", "bool":
		return a.V
	case "null":
		return nil
	case "undefined":
		return Undefined
	case "ident":
		return Ident(a.V.(string))
	case "idents":
		raw := a.V.([]any)
		return Idents(raw...)
	case "raw":
		return Raw(a.V.(string))
	case "in":
		return In(a.V.([]any)...)
	case "date":
		// Epoch milliseconds, in UTC. formatDate reads a time through its
		// location on both sides, and the reference's getFullYear() and friends
		// are local-time getters — which is why the runner pins TZ=UTC (H-18).
		return time.UnixMilli(argInt64(a.V)).UTC()
	case "blob":
		if a.V == nil {
			// Reaches "sql.blob() requires a Buffer or Uint8Array" on both
			// sides: a nil slice here, a non-Uint8Array there.
			return Blob(nil)
		}
		return Blob(jsonBytes(a.V))
	case "rawblob":
		// A []byte as a plain value, not through Blob: the reference refuses it
		// with a message the corpus never used to reach.
		return jsonBytes(a.V)
	case "unsupported":
		// Reaches "Unsupported value type: …", the one message sameError is
		// deliberately loose about — JavaScript reports typeof, Go reports its
		// own type name.
		return struct{ X int }{}
	}
	panic("unknown arg tag " + a.T)
}

// argInt64 reads a numeric argument. The generated corpus holds Go values, but
// the same tag has to work for a case that has been round-tripped through the
// case file, where every number is a float64.
func argInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	}
	panic(fmt.Sprintf("date argument is %T, want a number", v))
}

// jsonBytes reads a blob argument. The bytes travel as a JSON array of numbers
// so that both sides construct the same thing from the same literal; base64
// would make the case file unreadable for no gain.
func jsonBytes(v any) []byte {
	raw := v.([]any)
	out := make([]byte, len(raw))
	for i, n := range raw {
		switch x := n.(type) {
		case int:
			out[i] = byte(x)
		case float64:
			out[i] = byte(x)
		}
	}
	return out
}

// --- corpus generation -----------------------------------------------------

// generateCases builds a deterministic corpus. The runner constructs r from the
// seed and the stream, so a failure is reproducible from the seed alone and the
// same corpus can be replayed against a later version of either engine.
//
// The body is unchanged from before the runner was extracted, deliberately: the
// corpus bytes per seed are pinned by TestCorpusBytesAreUnchanged.
func generateCases(r *rand.Rand, n int) []diffCase {
	cases := make([]diffCase, 0, n)
	for i := range n {
		if i%2 == 0 {
			cases = append(cases, genFilterCase(r))
		} else {
			cases = append(cases, genSQLCase(r))
		}
	}
	return cases
}

// fieldNames mixes valid, invalid and JSON-path names so the identifier and
// path validation paths are exercised as hard as the happy path.
//
// Pure INTEGER keys ("0", "7") used to be excluded here: JavaScript's
// [[OwnPropertyKeys]] emits array-index keys first in ascending numeric order,
// so JSON.parse hands the reference a different key order than the one on the
// wire, and sqlb's own decoder did not reproduce that. Since decoding moved to
// jsorder (D-0023) it does, so the exclusion is gone — which is what proves the
// rule is now implemented rather than merely avoided.
var fieldNames = []string{
	"status", "age", "user_name", "_private", "A1",
	"profile.city", "a.b.c", "metadata.deleted", "deep.a.b.c.d",
	"user-name", "user name", "user.", ".name", "a..b", "",
	"name--", "name'", `name"`, "name;", "name)", "SELECT", "ORDER",
	"a.b--", "a.'b", "a. b", strings.Repeat("x", 300), strings.Repeat("y", 255),
	"0", "7", "10", "2", "01", "4294967295", "-1",
}

var aliasNames = []string{"t2", "orders", "_x", "users", "1bad", "bad-alias", "a.b", "", "x"}

var opNames = []string{
	"gt", "gte", "lt", "lte", "ne", "in", "nin", "like", "ilike", "regex",
	"exists", "and", "or", "unknown", "GT", "$gt",
}

// bigNumbers are literals JSON.parse cannot represent exactly. They exist in
// the corpus because the bound value has to match what the reference binds
// AFTER that rounding, not the digits on the wire — see DIVERGENCES J-01.
var bigNumbers = []json.RawMessage{
	json.RawMessage("9007199254740993"),         // 2^53+1, rounds DOWN in JSON.parse
	json.RawMessage("1234567890123456789"),      // rounds to …800
	json.RawMessage("12345678901234567890"),     // past int64 entirely
	json.RawMessage("-9007199254740993"),        //
	json.RawMessage("1e21"),                     // where JS flips to exponent form
	json.RawMessage("0.1"),                      //
	json.RawMessage("1.0"),                      // integral: must bind as 1, not 1.0
	json.RawMessage("-0"),                       //
	json.RawMessage("1e-7"),                     // where JS flips the other way
	json.RawMessage("9223372036854775807"),      // int64 max, not representable
	json.RawMessage("123456789012345678901234"), //
}

// genFilterScalar is genScalar plus the number literals only the FILTER path
// can carry. The SQL path cannot: its arguments are Go values handed straight
// to Q, so a raw JSON literal has no meaning there.
func genFilterScalar(r *rand.Rand) any {
	if r.IntN(12) == 0 {
		return bigNumbers[r.IntN(len(bigNumbers))]
	}
	return genScalar(r)
}

func genScalar(r *rand.Rand) any {
	switch r.IntN(9) {
	case 0:
		return nil
	case 1:
		return true
	case 2:
		return false
	case 3:
		return r.IntN(1000)
	case 4:
		return float64(r.IntN(1000)) + 0.5
	case 5:
		return ""
	case 6:
		return "'; DROP TABLE users; --"
	case 7:
		return strings.Repeat("p", r.IntN(1200))
	default:
		return fmt.Sprintf("v%d", r.IntN(100))
	}
}

func genOpValue(r *rand.Rand, op string) any {
	switch op {
	case "in", "nin":
		switch r.IntN(5) {
		case 0:
			return []any{}
		case 1:
			return "not an array"
		case 2:
			items := make([]any, r.IntN(4)+1)
			for i := range items {
				items[i] = genFilterScalar(r)
			}
			return items
		case 3:
			items := make([]any, 1000)
			for i := range items {
				items[i] = i
			}
			return items
		default:
			return []any{1, 2, 3}
		}
	case "exists":
		return genFilterScalar(r)
	case "like", "ilike", "regex":
		if r.IntN(4) == 0 {
			return genFilterScalar(r)
		}
		return "%" + fmt.Sprintf("p%d", r.IntN(50)) + "%"
	}
	return genFilterScalar(r)
}

// genFilterNode builds a filter as an ordered list of key/value pairs, then
// serialises it. It writes JSON by hand rather than marshalling a map so that
// key ORDER survives — which is the whole point of the exercise.
func genFilterNode(r *rand.Rand, depth int) string {
	var b strings.Builder
	b.WriteByte('{')
	entries := r.IntN(3) + 1
	for i := range entries {
		if i > 0 {
			b.WriteByte(',')
		}
		switch pick := r.IntN(10); {
		case pick < 2 && depth < 4:
			key := "and"
			if r.IntN(2) == 0 {
				key = "or"
			}
			writeJSONString(&b, key)
			b.WriteByte(':')
			if r.IntN(8) == 0 {
				b.WriteString("[]")
				break
			}
			b.WriteByte('[')
			subs := r.IntN(2) + 1
			for j := range subs {
				if j > 0 {
					b.WriteByte(',')
				}
				b.WriteString(genFilterNode(r, depth+1))
			}
			b.WriteByte(']')
		case pick < 3 && depth < 4:
			writeJSONString(&b, "$"+aliasNames[r.IntN(len(aliasNames))])
			b.WriteByte(':')
			b.WriteString(genFilterNode(r, depth+1))
		case pick < 6:
			writeJSONString(&b, fieldNames[r.IntN(len(fieldNames))])
			b.WriteByte(':')
			writeJSONValue(&b, genFilterScalar(r))
		default:
			writeJSONString(&b, fieldNames[r.IntN(len(fieldNames))])
			b.WriteString(":{")
			ops := r.IntN(3) + 1
			seen := map[string]bool{}
			written := 0
			for range ops {
				op := opNames[r.IntN(len(opNames))]
				if seen[op] {
					continue
				}
				seen[op] = true
				if written > 0 {
					b.WriteByte(',')
				}
				written++
				writeJSONString(&b, op)
				b.WriteByte(':')
				writeJSONValue(&b, genOpValue(r, op))
			}
			b.WriteByte('}')
		}
	}
	b.WriteByte('}')
	return b.String()
}

func writeJSONString(b *strings.Builder, s string) {
	enc, _ := json.Marshal(s)
	b.Write(enc)
}

func writeJSONValue(b *strings.Builder, v any) {
	enc, _ := json.Marshal(v)
	b.Write(enc)
}

func genFilterCase(r *rand.Rand) diffCase {
	return diffCase{Kind: "filter", Filter: json.RawMessage(genFilterNode(r, 0))}
}

// literalChunks are the static parts of a query. None contains a `?` in code
// position, since that is what Q reads as an interpolation slot; a few put one
// inside a literal or comment on purpose, to prove the slot scanner agrees with
// the placeholder scanner.
var literalChunks = []string{
	"SELECT * FROM users WHERE id = ",
	" AND name = ",
	"", " ", " LIMIT ", "; ", " -- trailing\n",
	" /* note */ ", " AND s = 'lit?eral' AND x = ", " AND [br?acket] = ",
	" AND q = \"do?uble\" AND ", " ; DROP TABLE users -- ",
	" AND a = 'it''s' AND ",
}

// closedChunks are literalChunks that leave no literal or comment open, so a
// `?` appended after one lands in code position.
var closedChunks = []string{
	"SELECT * FROM users WHERE id = ",
	" AND name = ",
	"", " ", " LIMIT ", "; ", " -- trailing\n",
	" /* note */ ", " AND s = 'lit?eral' AND x = ", " AND [br?acket] = ",
	" AND q = \"do?uble\" AND ", " AND a = 'it''s' AND ",
}

var rawChunks = []string{
	"CURRENT_TIMESTAMP", "COUNT(*)", "?", "? OR 1=1 --", "'; '", "/* ; */",
	"1=1", "", "NULL", "-- x", "[a?]", "`c?`", "'a''b?'",
}

func genSQLCase(r *rand.Rand) diffCase {
	nargs := r.IntN(4)
	parts := make([]string, nargs+1)
	for i := range parts {
		// Only the LAST chunk may leave a literal or comment open. Q marks an
		// interpolation with a `?` in CODE position, so an interpolation point
		// inside an unterminated comment is not expressible in the Go API at all
		// — see DIVERGENCES.md D-SLOT-IN-LITERAL. Generating one would compare
		// two different questions.
		if i == len(parts)-1 {
			parts[i] = literalChunks[r.IntN(len(literalChunks))]
		} else {
			parts[i] = closedChunks[r.IntN(len(closedChunks))]
		}
	}
	args := make([]diffArg, nargs)
	for i := range args {
		args[i] = genArg(r)
	}
	return diffCase{Kind: "sql", Parts: parts, Args: args}
}

func genArg(r *rand.Rand) diffArg {
	switch r.IntN(12) {
	case 0:
		return diffArg{T: "null"}
	case 1:
		return diffArg{T: "undefined"}
	case 2:
		return diffArg{T: "bool", V: r.IntN(2) == 0}
	case 3:
		return diffArg{T: "num", V: r.IntN(10000)}
	case 4:
		return diffArg{T: "ident", V: fieldNames[r.IntN(len(fieldNames))]}
	case 5:
		items := make([]any, r.IntN(3)+1)
		for i := range items {
			if r.IntN(3) == 0 {
				items[i] = fieldNames[r.IntN(len(fieldNames))]
			} else {
				items[i] = fmt.Sprintf("c%d", r.IntN(20))
			}
		}
		return diffArg{T: "idents", V: items}
	case 6:
		return diffArg{T: "raw", V: rawChunks[r.IntN(len(rawChunks))]}
	case 7:
		items := make([]any, r.IntN(4)+1)
		for i := range items {
			items[i] = genScalar(r)
		}
		return diffArg{T: "in", V: items}
	default:
		v := genScalar(r)
		if v == nil {
			return diffArg{T: "null"}
		}
		switch v.(type) {
		case bool:
			return diffArg{T: "bool", V: v}
		case int:
			return diffArg{T: "num", V: v}
		case float64:
			return diffArg{T: "num", V: v}
		}
		return diffArg{T: "str", V: v}
	}
}

// --- the value-type corpus -------------------------------------------------

// vtypeDates are the instants the date tag draws from, as epoch milliseconds.
//
// Years 1 and 999 are the point: formatDate does NOT zero-pad the year, because
// the reference builds it with String(date.getFullYear()), so 999 formats as
// "999-01-02" where Go's "2006" layout would give "0999". That claim was in a
// comment marked "confirmed against the reference" and had never been executed
// against it. The rest cover padStart on every other field, the epoch, a
// pre-epoch instant (negative milliseconds), and a sub-second component
// formatDate must drop.
var vtypeDates = []int64{
	time.Date(1, 1, 2, 3, 4, 5, 0, time.UTC).UnixMilli(),
	time.Date(999, 1, 2, 3, 4, 5, 0, time.UTC).UnixMilli(),
	time.Date(1000, 12, 31, 23, 59, 59, 0, time.UTC).UnixMilli(),
	time.Date(1969, 7, 20, 20, 17, 40, 0, time.UTC).UnixMilli(),
	time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli(),
	// Midnight is the TZ-sensitive one: under a non-UTC runtime the reference's
	// local-time getters move it to the previous day (H-18).
	time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC).UnixMilli(),
	time.Date(2026, 9, 12, 5, 6, 7, 500_000_000, time.UTC).UnixMilli(),
	time.Date(9999, 10, 11, 12, 13, 14, 0, time.UTC).UnixMilli(),
}

var vtypeBlobs = [][]any{
	{},
	{0},
	{1, 2, 255},
	{0, 13, 10, 39, 34, 59, 0x80, 0xff},
}

func generateValueTypeCases(r *rand.Rand, n int) []diffCase {
	cases := make([]diffCase, 0, n)
	for range n {
		nargs := r.IntN(3) + 1
		parts := make([]string, nargs+1)
		for i := range parts {
			// Every chunk is closed, so every `?` lands in code position: this
			// corpus is about argument VALUES, and the slot scanner is what the
			// main corpus already covers.
			parts[i] = closedChunks[r.IntN(len(closedChunks))]
		}
		args := make([]diffArg, nargs)
		for i := range args {
			args[i] = genValueTypeArg(r)
		}
		cases = append(cases, diffCase{Kind: "sql", Parts: parts, Args: args})
	}
	return cases
}

func genValueTypeArg(r *rand.Rand) diffArg {
	switch r.IntN(10) {
	case 0, 1, 2:
		return diffArg{T: "date", V: vtypeDates[r.IntN(len(vtypeDates))]}
	case 3, 4:
		return diffArg{T: "blob", V: vtypeBlobs[r.IntN(len(vtypeBlobs))]}
	case 5:
		// sql.blob(null): "sql.blob() requires a Buffer or Uint8Array".
		return diffArg{T: "blob"}
	case 6:
		// A Uint8Array as a plain value: "Buffer/Uint8Array values must be used
		// with sql.blob() for safe BLOB handling".
		return diffArg{T: "rawblob", V: vtypeBlobs[r.IntN(len(vtypeBlobs))]}
	case 7:
		return diffArg{T: "unsupported"}
	case 8:
		return diffArg{T: "num", V: r.IntN(10000)}
	default:
		return diffArg{T: "str", V: fmt.Sprintf("v%d", r.IntN(100))}
	}
}

// --- the error-path corpus -------------------------------------------------

// TestDifferentialFilterErrorPaths is a THIRD corpus: a fixed list of hostile
// shapes, each aimed at an error path the generated corpus cannot reach.
//
// The generated corpus reaches 24 of the reference's error messages. It cannot
// reach the rest BY CONSTRUCTION — genFilterNode never emits an array as a
// field value, never a non-object filter, never nesting past its own depth cap
// — so a one-character change to any of those messages is invisible at any
// number of cases. Deleting the trailing "." from "Field '%s' cannot have array
// value. Use logical operators 'and'/'or' instead." (compile.go, verbatim from
// src/filter.ts:365) left 25,000 cases green.
//
// A fixed list rather than a generator: these are specific shapes, there is
// nothing to randomise, and the count is then exact enough for the coverage
// floor to be an equality rather than a guess. No EnvPrefix, because the corpus
// is the list.
func TestDifferentialFilterErrorPaths(t *testing.T) {
	cases := errorPathCases()
	cov := &coverage{
		wantLabels: map[string]int{"error": 2},
		// Every distinct message this list is built to reach. An equality in
		// spirit: if a case stops reaching its path, this fails.
		minShapes: 11,
		// The nested-object value reaches "Unsupported value type:" through the
		// FILTER path, so the loose comparison is exercised here too.
		minLooseErrors: 1,
	}
	t.Cleanup(func() { t.Log(cov.summary()) })

	differential.CheckCorpus(t, differential.CorpusSpec[diffCase, diffResult]{
		Name: "sqlb-error-paths",
		Reference: differential.Reference{
			Script:       "../differential/run.ts",
			Dir:          "../..",
			Runtimes:     []string{"bun"},
			RequireFiles: []string{"src/sql.ts"},
		},
		Cases:      len(cases),
		Generate:   func(_ *rand.Rand, _ int) []diffCase { return cases },
		Kind:       func(c diffCase) string { return c.Kind },
		Eval:       func(c diffCase) diffResult { r, _ := runCase(c); return r },
		Compare:    cov.compare,
		Classifier: cov,
	})
}

// errorPathCases builds the fixed hostile corpus.
//
// Deliberately NOT here: a filter that compiles to no conditions at all
// ({}, {"$alias":"not an object"}). Since D-0032 that is an INTENDED divergence
// — Go compiles it to no WHERE clause, the reference throws "Filter must
// contain at least one condition" — and runCase reports it as a corpus
// invariant violation rather than a mismatch.
func errorPathCases() []diffCase {
	filters := []string{
		// Field '…' cannot have array value. Use logical operators … instead.
		`{"status":["a","b"]}`,
		// Filter must be an object, three ways.
		`"not an object"`,
		`null`,
		`123`,
		// An array's keys are its indices, and JavaScript's
		// [[OwnPropertyKeys]] hands them over as strings — so this reaches the
		// identifier validator with "0", not a type error.
		`[1,2]`,
		// Unsupported value type: the reference reports typeof (object), Go
		// reports its own type name. The one place sameError is loose.
		`{"a":{"gt":{"nested":1}}}`,
		// AND/OR operator must be an array.
		`{"and":"not an array"}`,
		`{"or":{"a":1}}`,
		// Operator object must contain at least one valid operator.
		`{"a":{}}`,
		// Nesting depth too deep (max: 10).
		strings.Repeat(`{"and":[`, 12) + `{"a":1}` + strings.Repeat(`]}`, 12),
		// Too many operators (max: 100).
		wideFilter(150),
		// Compiled filter too long (max: 102400 bytes): 60 conditions of 999
		// placeholders each, which is under the operator cap and over the byte
		// cap.
		bigInFilter(60, 999),
	}
	cases := make([]diffCase, 0, len(filters)+1)
	for _, f := range filters {
		cases = append(cases, diffCase{Kind: "filter", Filter: json.RawMessage(f)})
	}
	// Query too long, on the sql path. Both sides measure their own assembled
	// text, and the text is identical, so the byte count in the message is too.
	cases = append(cases, diffCase{
		Kind:  "sql",
		Parts: []string{strings.Repeat("x", MaxQueryLength+1)},
		Args:  []diffArg{},
	})
	return cases
}

func wideFilter(n int) string {
	var b strings.Builder
	b.WriteByte('{')
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `"f%d":%d`, i, i)
	}
	b.WriteByte('}')
	return b.String()
}

func bigInFilter(conditions, items int) string {
	var b strings.Builder
	b.WriteByte('{')
	for i := range conditions {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `"f%d":{"in":[`, i)
		for j := range items {
			if j > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, "%d", j)
		}
		b.WriteString("]}")
	}
	b.WriteByte('}')
	return b.String()
}
