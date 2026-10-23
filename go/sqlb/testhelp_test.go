package sqlb

import (
	"context"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

// captureWarnings collects the messages sqlb logs at warn level while fn runs.
// The reference asserts on console.warn (src/sql.test.ts:713); slog is the
// closest thing a Go library has, and this is how the assertion survives.
type warnCollector struct {
	slog.Handler
	msgs *[]string
}

func (w warnCollector) Handle(_ context.Context, r slog.Record) error {
	if r.Level >= slog.LevelWarn {
		*w.msgs = append(*w.msgs, r.Message)
	}
	return nil
}

func (w warnCollector) Enabled(context.Context, slog.Level) bool { return true }

func captureWarnings(t *testing.T, fn func()) []string {
	t.Helper()
	var msgs []string
	prev := slog.Default()
	slog.SetDefault(slog.New(warnCollector{Handler: prev.Handler(), msgs: &msgs}))
	defer slog.SetDefault(prev)
	fn()
	return msgs
}

// eqValues compares a bound-value slice the way vitest's toEqual does: an empty
// slice and a nil slice are the same thing.
func eqValues(got []any, want []any) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	return reflect.DeepEqual(got, want)
}

func wantValues(t *testing.T, got []any, want ...any) {
	t.Helper()
	if !eqValues(got, want) {
		t.Errorf("values = %#v, want %#v", got, want)
	}
}

func wantText(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("text  = %q\n want = %q", got, want)
	}
}

// wantOK asserts a fragment built cleanly and returns it.
func wantOK(t *testing.T, f Fragment) Fragment {
	t.Helper()
	if f.Err() != nil {
		t.Fatalf("unexpected error: %v", f.Err())
	}
	return f
}

// wantFragErr asserts a fragment carries an error whose message contains substr.
func wantFragErr(t *testing.T, f Fragment, substr string) {
	t.Helper()
	if f.Err() == nil {
		t.Fatalf("expected error containing %q, got text %q", substr, f.Text())
	}
	if !strings.Contains(f.Err().Error(), substr) {
		t.Errorf("error = %q, want it to contain %q", f.Err(), substr)
	}
}

// wantErr asserts err is non-nil and mentions substr.
func wantErr(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error = %q, want it to contain %q", err, substr)
	}
}

func wantNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// countRunes counts occurrences of a byte, standing in for the
// (text.match(/\?/g) ?? []).length the reference tests use.
func countByte(s string, b byte) int {
	n := 0
	for i := range len(s) {
		if s[i] == b {
			n++
		}
	}
	return n
}
