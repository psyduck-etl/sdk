package data

import (
	"errors"
	"testing"
)

// ── Get() boundaries ─────────────────────────────────────────────────────
//
// Bytes.Get and Runes.Get had no direct test at all before this file (only
// Str.Get and, indirectly through Walk, List.Get were exercised, and even
// those only at non-boundary indices). Each Get is `n < 0 || n >= len(x)`;
// n == 0 and n == len(x) are the exact inputs that distinguish that from a
// weakened `<=`/`>` boundary mutant.

func TestBytesGetBoundaries(t *testing.T) {
	b := Bytes("abc")
	if g, ok := b.Get(0); !ok || g.String() != "a" {
		t.Errorf("Get(0) = %q ok=%v, want a true", g, ok)
	}
	if g, ok := b.Get(2); !ok || g.String() != "c" {
		t.Errorf("Get(2) = %q ok=%v, want c true", g, ok)
	}
	if _, ok := b.Get(-1); ok {
		t.Error("Get(-1) should be out of range")
	}
	if _, ok := b.Get(3); ok { // == len(b): one past the end
		t.Error("Get(len) should be out of range")
	}
}

func TestRunesGetBoundaries(t *testing.T) {
	r := Runes("abc")
	if g, ok := r.Get(0); !ok || g.String() != "a" {
		t.Errorf("Get(0) = %q ok=%v, want a true", g, ok)
	}
	if g, ok := r.Get(2); !ok || g.String() != "c" {
		t.Errorf("Get(2) = %q ok=%v, want c true", g, ok)
	}
	if _, ok := r.Get(-1); ok {
		t.Error("Get(-1) should be out of range")
	}
	if _, ok := r.Get(3); ok {
		t.Error("Get(len) should be out of range")
	}
}

func TestStrGetBoundaries(t *testing.T) {
	s := Str("abc")
	if g, ok := s.Get(0); !ok || g.String() != "a" {
		t.Errorf("Get(0) = %q ok=%v, want a true", g, ok)
	}
	if g, ok := s.Get(2); !ok || g.String() != "c" {
		t.Errorf("Get(2) = %q ok=%v, want c true", g, ok)
	}
	if _, ok := s.Get(-1); ok {
		t.Error("Get(-1) should be out of range")
	}
	if _, ok := s.Get(3); ok {
		t.Error("Get(len) should be out of range")
	}
}

func TestListGetBoundaries(t *testing.T) {
	// List.Get returns a length-1 List (not the bare element), so it renders
	// as a one-element JSON array.
	l := List{Str("a"), Str("b"), Str("c")}
	if g, ok := l.Get(0); !ok || g.String() != `["a"]` {
		t.Errorf("Get(0) = %q ok=%v, want %s true", g, ok, `["a"]`)
	}
	if g, ok := l.Get(2); !ok || g.String() != `["c"]` {
		t.Errorf("Get(2) = %q ok=%v, want %s true", g, ok, `["c"]`)
	}
	if _, ok := l.Get(-1); ok {
		t.Error("Get(-1) should be out of range")
	}
	if _, ok := l.Get(3); ok {
		t.Error("Get(len) should be out of range")
	}
}

// ── By() ──────────────────────────────────────────────────────────────────
//
// applyBy's error check (`if err != nil`) was NOT COVERED: nothing called
// .By on any Continuous type. Exercise both the success and failure path
// across all four concrete types.

var errByPattern = errors.New("pattern failed")

func upperBytePattern(b []byte) ([]byte, error) {
	out := make([]byte, len(b))
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		out[i] = c
	}
	return out, nil
}

func failBytePattern(b []byte) ([]byte, error) { return nil, errByPattern }

func TestContinuousByAppliesPattern(t *testing.T) {
	cases := []Continuous{Bytes("abc"), Runes("abc"), Str("abc"), List{Str("a")}}
	for _, c := range cases {
		out, err := c.By(upperBytePattern)
		if err != nil {
			t.Fatalf("%T By: %v", c, err)
		}
		if _, ok := out.(Bytes); !ok {
			t.Errorf("%T By result = %T, want Bytes", c, out)
		}
	}
}

func TestContinuousByPropagatesError(t *testing.T) {
	cases := []Continuous{Bytes("abc"), Runes("abc"), Str("abc"), List{Str("a")}}
	for _, c := range cases {
		if _, err := c.By(failBytePattern); !errors.Is(err, errByPattern) {
			t.Errorf("%T By error = %v, want %v", c, err, errByPattern)
		}
	}
}
