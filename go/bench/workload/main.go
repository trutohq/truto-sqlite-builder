// Command workload is the Go side of the JS-versus-Go comparison. It performs
// exactly the work go/bench/workload.ts performs and reports the same numbers.
//
//	go run ./bench/workload <iterations>
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/trutohq/truto-sqlite-builder/go/sqlb"
)

var columns = []any{
	"id", "name", "email", "status", "created_at", "updated_at", "u.role", "p.title",
}

// The same filter as workload.ts, as the JSON a service would receive.
var filterJSON = []byte(`{"and":[{"available":true},{"price":{"gte":10,"lte":500}},{"or":[{"and":[{"category":"electronics"},{"or":[{"brand":{"in":["Apple","Samsung","Google"]}},{"and":[{"brand":"Other"},{"rating":{"gte":4.5}},{"reviews_count":{"gte":100}}]}]}]},{"and":[{"category":"clothing"},{"or":[{"and":[{"attributes.gender":"unisex"},{"attributes.size":{"in":["S","M","L"]}}]},{"and":[{"attributes.style":{"like":"%casual%"}},{"discount_percentage":{"gte":20}}]}]}]},{"and":[{"category":"books"},{"or":[{"genre":{"in":["fiction","mystery","sci-fi"]}},{"and":[{"genre":"non-fiction"},{"metadata.bestseller":true}]}]},{"language":{"in":["english","spanish"]}}]}]}]}`)

func once(i int) (int, error) {
	filter, err := sqlb.DecodeFilter(filterJSON)
	if err != nil {
		return 0, err
	}
	where, err := sqlb.CompileFilter(filter)
	if err != nil {
		return 0, err
	}
	query, err := sqlb.Q(`
    SELECT ?
    FROM ? p
    JOIN ? u ON u.id = p.owner_id
    WHERE ?
    ORDER BY ? DESC
    LIMIT ? OFFSET ?
  `,
		sqlb.Idents(columns...),
		sqlb.Ident("products"),
		sqlb.Ident("users"),
		where,
		sqlb.Ident("p.created_at"),
		100,
		i%1000,
	)
	if err != nil {
		return 0, err
	}
	return len(query.Text()) + len(query.Values()), nil
}

func main() {
	iterations := 200000
	if len(os.Args) > 1 {
		n, err := strconv.Atoi(os.Args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "usage: workload <iterations>")
			os.Exit(2)
		}
		iterations = n
	}

	checksum := 0
	// Match the JavaScript's warm-up so both measure steady state.
	for i := range 2000 {
		n, err := once(i)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		checksum += n
	}

	start := time.Now()
	for i := range iterations {
		n, err := once(i)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		checksum += n
	}
	elapsed := time.Since(start)

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	out, _ := json.Marshal(map[string]any{
		"runtime":        "go " + runtime.Version(),
		"iterations":     iterations,
		"elapsed_ms":     float64(elapsed.Microseconds()) / 1000,
		"ns_per_op":      float64(elapsed.Nanoseconds()) / float64(iterations),
		"peak_rss_bytes": peakRSS(),
		"go_heap_sys":    ms.HeapSys,
		"checksum":       checksum,
	})
	fmt.Println(string(out))
}

// peakRSS reads the high-water mark the kernel recorded, which is the only
// number comparable with what process.memoryUsage().rss reports on the other
// side. Falls back to 0 where /proc is unavailable.
func peakRSS() uint64 {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	const key = "VmHWM:"
	for line := range splitLines(string(data)) {
		if len(line) > len(key) && line[:len(key)] == key {
			var kb uint64
			fmt.Sscanf(line[len(key):], "%d", &kb)
			return kb * 1024
		}
	}
	return 0
}

func splitLines(s string) func(func(string) bool) {
	return func(yield func(string) bool) {
		start := 0
		for i := range len(s) {
			if s[i] == '\n' {
				if !yield(s[start:i]) {
					return
				}
				start = i + 1
			}
		}
		if start < len(s) {
			yield(s[start:])
		}
	}
}
