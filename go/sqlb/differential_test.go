// Differential testing against the TypeScript reference.
//
// goenvoy's recorded lesson from the same programme is that every real
// divergence it found came from EXECUTING both engines, and none from reading
// the TypeScript. So this generates a large deterministic corpus, runs it
// through bun and through this package, and diffs the results — text, values
// and error messages.
//
// It skips when bun or the reference checkout is unavailable, so `go test`
// still works standalone. Run it explicitly with:
//
//	go test -run TestDifferential -v ./...
package sqlb

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
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
}

func TestDifferential(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun not on PATH; skipping differential test")
	}
	runner, err := filepath.Abs("../differential/run.ts")
	if err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "src", "sql.ts")); err != nil {
		t.Skipf("reference checkout not found at %s; skipping", repoRoot)
	}

	n := 4000
	if v := os.Getenv("SQLB_DIFF_CASES"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("SQLB_DIFF_CASES: %v", err)
		}
		n = parsed
	}
	var seed uint64 = 0x5150
	if v := os.Getenv("SQLB_DIFF_SEED"); v != "" {
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			t.Fatalf("SQLB_DIFF_SEED: %v", err)
		}
		seed = parsed
	}
	cases := generateCases(n, seed)

	input, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(t.TempDir(), "cases.json")
	if err := os.WriteFile(inputPath, input, 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bun, runner, inputPath)
	cmd.Dir = repoRoot
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running the reference failed: %v", err)
	}

	var want []diffResult
	if err := json.Unmarshal(out, &want); err != nil {
		t.Fatalf("decoding reference output: %v", err)
	}
	if len(want) != len(cases) {
		t.Fatalf("reference returned %d results for %d cases", len(want), len(cases))
	}

	var mismatches, agreed, bothErrored int
	for i, c := range cases {
		got := runCase(t, c)
		w := want[i]

		if got.OK != w.OK {
			mismatches++
			if mismatches <= 20 {
				t.Errorf("case %d %s: go ok=%v (%s / %s), js ok=%v (%s)\n  input: %s",
					i, c.Kind, got.OK, got.Text, got.Error, w.OK, w.Error, describe(c))
			}
			continue
		}
		if !got.OK {
			bothErrored++
			if !sameError(got.Error, w.Error) {
				mismatches++
				if mismatches <= 20 {
					t.Errorf("case %d %s: error mismatch\n  go: %s\n  js: %s\n  input: %s",
						i, c.Kind, got.Error, w.Error, describe(c))
				}
			}
			continue
		}
		if got.Text != w.Text {
			mismatches++
			if mismatches <= 20 {
				t.Errorf("case %d %s: text mismatch\n  go: %s\n  js: %s\n  input: %s",
					i, c.Kind, got.Text, w.Text, describe(c))
			}
			continue
		}
		if gv, wv := canonValues(got.Values), canonValues(w.Values); gv != wv {
			mismatches++
			if mismatches <= 20 {
				t.Errorf("case %d %s: values mismatch\n  go: %s\n  js: %s\n  input: %s",
					i, c.Kind, gv, wv, describe(c))
			}
			continue
		}
		agreed++
	}

	t.Logf("differential: %d cases, %d agreed on output, %d agreed on the error, %d mismatched",
		len(cases), agreed, bothErrored-mismatches, mismatches)
	if mismatches > 0 {
		t.Fatalf("%d differential mismatches", mismatches)
	}
}

func describe(c diffCase) string {
	b, _ := json.Marshal(c)
	if len(b) > 400 {
		return string(b[:400]) + "…"
	}
	return string(b)
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

func runCase(t *testing.T, c diffCase) diffResult {
	t.Helper()
	switch c.Kind {
	case "filter":
		frag, err := CompileFilterJSON(c.Filter)
		if err != nil {
			return diffResult{Error: err.Error()}
		}
		return diffResult{OK: true, Text: frag.Text(), Values: frag.Values()}
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
			return diffResult{Error: err.Error()}
		}
		return diffResult{OK: true, Text: frag.Text(), Values: frag.Values()}
	}
	t.Fatalf("unknown case kind %q", c.Kind)
	return diffResult{}
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
	}
	panic("unknown arg tag " + a.T)
}

// --- corpus generation -----------------------------------------------------

// generateCases builds a deterministic corpus. The seed is fixed so a failure
// is reproducible and so the same corpus can be replayed against a later
// version of either engine.
func generateCases(n int, seed uint64) []diffCase {
	r := rand.New(rand.NewPCG(seed, 0x1979))
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
// path validation paths are exercised as hard as the happy path. Pure integer
// keys are deliberately absent: JavaScript's [[OwnPropertyKeys]] emits those
// first in numeric order, so JSON.parse would hand the reference a different
// key order than the one on the wire. See GOTCHAS.md.
var fieldNames = []string{
	"status", "age", "user_name", "_private", "A1",
	"profile.city", "a.b.c", "metadata.deleted", "deep.a.b.c.d",
	"user-name", "user name", "user.", ".name", "a..b", "",
	"name--", "name'", `name"`, "name;", "name)", "SELECT", "ORDER",
	"a.b--", "a.'b", "a. b", strings.Repeat("x", 300), strings.Repeat("y", 255),
}

var aliasNames = []string{"t2", "orders", "_x", "users", "1bad", "bad-alias", "a.b", "", "x"}

var opNames = []string{
	"gt", "gte", "lt", "lte", "ne", "in", "nin", "like", "ilike", "regex",
	"exists", "and", "or", "unknown", "GT", "$gt",
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
				items[i] = genScalar(r)
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
		return genScalar(r)
	case "like", "ilike", "regex":
		if r.IntN(4) == 0 {
			return genScalar(r)
		}
		return "%" + fmt.Sprintf("p%d", r.IntN(50)) + "%"
	}
	return genScalar(r)
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
			writeJSONValue(&b, genScalar(r))
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
