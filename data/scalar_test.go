package data

import (
	"math"
	"testing"
)

// Lit.Bytes falls back to sprint when json.Marshal errors, but nothing
// forced that error before: every Lit in the other tests holds a
// JSON-safe value. NaN is a syntactically valid Go float64 that
// json.Marshal refuses, so it exercises the fallback path.
func TestLitNaNFallback(t *testing.T) {
	l := Lit{math.NaN()}
	if got := l.String(); got != "NaN" {
		t.Errorf("NaN literal fallback = %q, want NaN", got)
	}
}
