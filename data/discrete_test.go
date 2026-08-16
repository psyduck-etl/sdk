package data

import "testing"

// Transpose's `i < len(dsts)` guard (discrete.go:79) had no test where srcs
// outnumber dsts, so the CONDITIONALS_BOUNDARY mutant (< -> <=) survived:
// with more resolved srcs than dsts, index i eventually reaches len(dsts),
// where the guard must skip the write rather than index out of dsts.
func TestTransposeMoreSrcsThanDsts(t *testing.T) {
	v, _ := Decode([]byte(`{"a":"1","b":"2","c":"3"}`), "json")
	out, missing := Transpose(v,
		[]Path{{"a"}, {"b"}, {"c"}},
		[]string{"x", "y"}, // fewer dsts than srcs
	)
	if len(out) != 2 || out["x"].String() != "1" || out["y"].String() != "2" {
		t.Errorf("transpose = %v, want x=1 y=2 only", out)
	}
	// The third src resolved fine; it just had nowhere to go, so it isn't
	// "missing" either.
	if len(missing) != 0 {
		t.Errorf("missing = %v, want none", missing)
	}
}
