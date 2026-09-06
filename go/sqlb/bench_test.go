package sqlb

import (
	"encoding/json"
	"strconv"
	"sync"
	"testing"
	"time"
)

// The benchmarks mirror bench/benchmark.ts where the reference has an
// equivalent, so the two can be compared directly, and add the filter compiler
// which the reference does not benchmark at all.
//
// allocs/op is the number to watch: it is stable across machines in a way
// ns/op on a shared box is not, and it is what separates a careful port from a
// mechanical one.

func BenchmarkQSimple(b *testing.B) {
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		q, err := Q("SELECT * FROM users WHERE id = ?", i)
		if err != nil {
			b.Fatal(err)
		}
		sink(q)
	}
}

func BenchmarkQComplex(b *testing.B) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		q, err := Q(`
      INSERT INTO users (name, email, age, created_at)
      VALUES (?, ?, ?, ?)
    `, "User 1", "user1@example.com", 37, now)
		if err != nil {
			b.Fatal(err)
		}
		sink(q)
	}
}

func BenchmarkQWithIn10(b *testing.B) {
	ids := make([]int, 10)
	for i := range ids {
		ids[i] = i + 1
	}
	b.ReportAllocs()
	for b.Loop() {
		q, err := Q("SELECT * FROM users WHERE id IN ?", InSlice(ids))
		if err != nil {
			b.Fatal(err)
		}
		sink(q)
	}
}

func BenchmarkIdent(b *testing.B) {
	tables := []string{"users", "posts", "comments", "categories", "tags"}
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		sink(Ident(tables[i%len(tables)]))
	}
}

func BenchmarkIdentQualified(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink(Ident("schema.table.column"))
	}
}

func BenchmarkIdents10(b *testing.B) {
	cols := []any{"id", "name", "email", "age", "created_at",
		"updated_at", "deleted_at", "u.role", "u.status", "p.title"}
	b.ReportAllocs()
	for b.Loop() {
		sink(Idents(cols...))
	}
}

func BenchmarkJoin10(b *testing.B) {
	frags := make([]Fragment, 10)
	for i := range frags {
		q, err := Q("col = ?", i)
		if err != nil {
			b.Fatal(err)
		}
		frags[i] = q
	}
	b.ReportAllocs()
	for b.Loop() {
		sink(Join(frags, " AND "))
	}
}

func BenchmarkScanSQLLarge(b *testing.B) {
	// A realistic large statement: a 200-column insert with comments and
	// literals, which is what the scanner actually meets in a batch write.
	text := largeStatement(200)
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for b.Loop() {
		p, s := scanSQL(text)
		if s || p != 200 {
			b.Fatalf("unexpected scan result %d %v", p, s)
		}
	}
}

func largeStatement(cols int) string {
	q := `/* batch insert */ INSERT INTO "events" (`
	for i := range cols {
		if i > 0 {
			q += ", "
		}
		q += `"c` + strconv.Itoa(i) + `"`
	}
	q += ") VALUES ("
	for i := range cols {
		if i > 0 {
			q += ","
		}
		q += "?"
	}
	return q + ") -- trailing note\n"
}

func BenchmarkCompileFilterSmall(b *testing.B) {
	f := NewFilter().Eq("status", "ACTIVE").Where("age", NewOps().Gte(18))
	b.ReportAllocs()
	for b.Loop() {
		frag, err := CompileFilter(f)
		if err != nil {
			b.Fatal(err)
		}
		sink(frag)
	}
}

func BenchmarkCompileFilterRealistic(b *testing.B) {
	f := realisticFilter()
	b.ReportAllocs()
	for b.Loop() {
		frag, err := CompileFilter(f)
		if err != nil {
			b.Fatal(err)
		}
		sink(frag)
	}
}

// realisticFilter is the e-commerce search from the reference's own test suite
// (src/filter.test.ts:590): three levels of and/or, JSON paths, IN lists and
// LIKE patterns. 30 bound values.
func realisticFilter() *Filter {
	return NewFilter().And(
		NewFilter().Eq("available", true),
		NewFilter().Where("price", NewOps().Gte(10).Lte(500)),
		NewFilter().Or(
			NewFilter().And(
				NewFilter().Eq("category", "electronics"),
				NewFilter().Or(
					NewFilter().Where("brand", NewOps().In("Apple", "Samsung", "Google")),
					NewFilter().And(
						NewFilter().Eq("brand", "Other"),
						NewFilter().Where("rating", NewOps().Gte(4.5)),
						NewFilter().Where("reviews_count", NewOps().Gte(100)),
					),
				),
			),
			NewFilter().And(
				NewFilter().Eq("category", "clothing"),
				NewFilter().Or(
					NewFilter().And(
						NewFilter().Eq("attributes.gender", "unisex"),
						NewFilter().Where("attributes.size", NewOps().In("S", "M", "L")),
					),
					NewFilter().And(
						NewFilter().Where("attributes.style", NewOps().Like("%casual%")),
						NewFilter().Where("discount_percentage", NewOps().Gte(20)),
					),
				),
			),
			NewFilter().And(
				NewFilter().Eq("category", "books"),
				NewFilter().Or(
					NewFilter().Where("genre", NewOps().In("fiction", "mystery", "sci-fi")),
					NewFilter().And(
						NewFilter().Eq("genre", "non-fiction"),
						NewFilter().Eq("metadata.bestseller", true),
					),
				),
				NewFilter().Where("language", NewOps().In("english", "spanish")),
			),
		),
	)
}

var realisticFilterJSON = []byte(`{"and":[{"available":true},{"price":{"gte":10,"lte":500}},{"or":[{"and":[{"category":"electronics"},{"or":[{"brand":{"in":["Apple","Samsung","Google"]}},{"and":[{"brand":"Other"},{"rating":{"gte":4.5}},{"reviews_count":{"gte":100}}]}]}]},{"and":[{"category":"clothing"},{"or":[{"and":[{"attributes.gender":"unisex"},{"attributes.size":{"in":["S","M","L"]}}]},{"and":[{"attributes.style":{"like":"%casual%"}},{"discount_percentage":{"gte":20}}]}]}]},{"and":[{"category":"books"},{"or":[{"genre":{"in":["fiction","mystery","sci-fi"]}},{"and":[{"genre":"non-fiction"},{"metadata.bestseller":true}]}]},{"language":{"in":["english","spanish"]}}]}]}]}`)

func BenchmarkCompileFilterJSONRealistic(b *testing.B) {
	b.SetBytes(int64(len(realisticFilterJSON)))
	b.ReportAllocs()
	for b.Loop() {
		frag, err := CompileFilterJSON(realisticFilterJSON)
		if err != nil {
			b.Fatal(err)
		}
		sink(frag)
	}
}

// BenchmarkFilterDecodeJSONOnly separates decoding from compiling, because a
// caller that reuses a decoded filter pays only the second.
func BenchmarkFilterDecodeJSONOnly(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		f := NewFilter()
		if err := json.Unmarshal(realisticFilterJSON, f); err != nil {
			b.Fatal(err)
		}
		sink(f)
	}
}

// BenchmarkQParallel proves the package holds no shared mutable state and
// measures how it scales. Nothing here is parallelised internally — see
// DECISIONS.md D-0104 — so the only question is whether a caller can run it
// concurrently, and this is the evidence that it can.
func BenchmarkQParallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q, err := Q("SELECT * FROM users WHERE id = ? AND name = ?", 42, "John")
			if err != nil {
				b.Fatal(err)
			}
			sink(q)
		}
	})
}

// BenchmarkBatchCompileSequential and BenchmarkBatchCompileParallel exist to
// find the input size at which handing a batch of filters to a worker pool
// beats a plain loop. The library does not do this itself; the numbers are in
// the README so a caller can decide.
func benchBatch(b *testing.B, n int, parallel bool) {
	filters := make([]*Filter, n)
	for i := range filters {
		filters[i] = realisticFilter()
	}
	out := make([]Fragment, n)
	b.ReportAllocs()
	for b.Loop() {
		if !parallel {
			for i, f := range filters {
				frag, err := CompileFilter(f)
				if err != nil {
					b.Fatal(err)
				}
				out[i] = frag
			}
			continue
		}
		var wg sync.WaitGroup
		workers := 8
		chunk := (n + workers - 1) / workers
		for w := range workers {
			start := w * chunk
			end := min(start+chunk, n)
			if start >= end {
				continue
			}
			wg.Add(1)
			go func(start, end int) {
				defer wg.Done()
				for i := start; i < end; i++ {
					frag, err := CompileFilter(filters[i])
					if err != nil {
						panic(err)
					}
					// Each worker writes its own indices: order is preserved, which
					// is non-negotiable when the output feeds a bound-value list.
					out[i] = frag
				}
			}(start, end)
		}
		wg.Wait()
	}
	sink(out[n-1])
}

func BenchmarkBatchCompile1Sequential(b *testing.B)    { benchBatch(b, 1, false) }
func BenchmarkBatchCompile1Parallel(b *testing.B)      { benchBatch(b, 1, true) }
func BenchmarkBatchCompile8Sequential(b *testing.B)    { benchBatch(b, 8, false) }
func BenchmarkBatchCompile8Parallel(b *testing.B)      { benchBatch(b, 8, true) }
func BenchmarkBatchCompile64Sequential(b *testing.B)   { benchBatch(b, 64, false) }
func BenchmarkBatchCompile64Parallel(b *testing.B)     { benchBatch(b, 64, true) }
func BenchmarkBatchCompile512Sequential(b *testing.B)  { benchBatch(b, 512, false) }
func BenchmarkBatchCompile512Parallel(b *testing.B)    { benchBatch(b, 512, true) }
func BenchmarkBatchCompile4096Sequential(b *testing.B) { benchBatch(b, 4096, false) }
func BenchmarkBatchCompile4096Parallel(b *testing.B)   { benchBatch(b, 4096, true) }

//go:noinline
func sink(v any) { sunk = v }

var sunk any
