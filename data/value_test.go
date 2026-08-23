package data

import "testing"

// Table test for Kind.Continuous() across every Kind value.
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
