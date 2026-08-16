package data

import "testing"

// Kind.Continuous() was never called by any test, so both of its `!=`
// comparisons were NOT COVERED.
func TestKindContinuous(t *testing.T) {
	want := map[Kind]bool{
		KindBytes:  true,
		KindStr:    true,
		KindRunes:  true,
		KindList:   true,
		KindObject: false,
		KindLit:    false,
	}
	for k, w := range want {
		if got := k.Continuous(); got != w {
			t.Errorf("%s.Continuous() = %v, want %v", k, got, w)
		}
	}
}
