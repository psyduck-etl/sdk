package data

import (
	"strconv"
	"testing"
)

// ── codec chains & round-trips ─────────────────────────────────────────────

func TestDecodeEncodeRoundTrip(t *testing.T) {
	// Invariant: Decode(Encode(v, spec), spec) == v. The initial Value is built
	// from a natural source ("json" text, raw bytes) then round-tripped through
	// the target spec so byte codecs (base64/hex/gzip) see a real payload.
	cases := []struct {
		name       string
		source     []byte
		sourceSpec string
		spec       string
	}{
		{"bytes", []byte{0x00, 0x01, 0xff}, "bytes", "bytes"},
		{"utf8", []byte("héllo"), "utf-8", "utf-8"},
		{"json-object", []byte(`{"a":1,"b":"two","c":[1,2,3]}`), "json", "json"},
		{"base64", []byte("hello world"), "bytes", "base64"},
		{"hex", []byte("hello"), "bytes", "hex"},
		{"gzip-json", []byte(`{"k":"v"}`), "json", "gzip|json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := Decode(c.source, c.sourceSpec)
			if err != nil {
				t.Fatalf("seed Decode(%q): %v", c.sourceSpec, err)
			}
			out, err := Encode(v, c.spec)
			if err != nil {
				t.Fatalf("Encode(%q): %v", c.spec, err)
			}
			v2, err := Decode(out, c.spec)
			if err != nil {
				t.Fatalf("Decode(%q): %v", c.spec, err)
			}
			if string(v.Bytes()) != string(v2.Bytes()) {
				t.Errorf("round-trip mismatch: %q -> %q", v.Bytes(), v2.Bytes())
			}
		})
	}
}

func TestJSONNumberFidelity(t *testing.T) {
	v, err := Decode([]byte(`{"n":42,"f":3.5,"b":true,"z":null}`), "json")
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.Bytes())
	want := `{"b":true,"f":3.5,"n":42,"z":null}`
	if got != want {
		t.Errorf("number/bool/null not preserved: got %s want %s", got, want)
	}
}

// ── text transformers on garbage / edge input ─────────────────────────────

func TestUTF8RejectsGarbage(t *testing.T) {
	// invalid continuation byte, lone 0xff, truncated multibyte
	garbage := [][]byte{
		{0xff},
		{0xc3, 0x28},     // invalid 2-byte seq
		{0xe2, 0x82},     // truncated 3-byte (partial rune)
		{'a', 0x80, 'b'}, // stray continuation
	}
	for i, g := range garbage {
		if _, err := Decode(g, "utf-8"); err == nil {
			t.Errorf("case %d: expected utf-8 decode error for % x", i, g)
		}
	}
}

func TestASCIIRejectsHighBytes(t *testing.T) {
	if _, err := Decode([]byte{'a', 0x80}, "ascii"); err == nil {
		t.Error("expected ascii error on high byte")
	}
	if _, err := Decode([]byte("plain"), "ascii"); err != nil {
		t.Errorf("valid ascii rejected: %v", err)
	}
}

func TestLatin1AcceptsAllBytes(t *testing.T) {
	// latin1 maps every byte to a rune, so nothing is invalid.
	v, err := Decode([]byte{0xff, 0x80, 0x41}, "latin1")
	if err != nil {
		t.Fatalf("latin1 should accept all bytes: %v", err)
	}
	if v.Kind() != KindStr {
		t.Errorf("latin1 kind = %s, want string", v.Kind())
	}
	if got := []rune(v.String()); len(got) != 3 {
		t.Errorf("latin1 rune count = %d, want 3", len(got))
	}
}

func TestEmptyInput(t *testing.T) {
	for _, spec := range []string{"bytes", "utf-8", "ascii", "latin1"} {
		if _, err := Decode([]byte{}, spec); err != nil {
			t.Errorf("empty input under %q errored: %v", spec, err)
		}
	}
}

// ── continuous ops ────────────────────────────────────────────────────────

func TestBytesSlice(t *testing.T) {
	b := Bytes("0123456789")
	out, trunc := b.Slice(2, 6, 1)
	if string(out.Bytes()) != "2345" || trunc {
		t.Errorf("slice = %q trunc=%v, want 2345 false", out.Bytes(), trunc)
	}
	out, trunc = b.Slice(0, 100, 1) // overflow clamps
	if string(out.Bytes()) != "0123456789" || !trunc {
		t.Errorf("overflow slice = %q trunc=%v, want full true", out.Bytes(), trunc)
	}
	out, _ = b.Slice(0, 10, 2) // step
	if string(out.Bytes()) != "02468" {
		t.Errorf("step slice = %q, want 02468", out.Bytes())
	}
	out, trunc = b.Slice(-3, -1, 1) // negative bounds clamp to 0, not from-the-end
	if string(out.Bytes()) != "" || !trunc {
		t.Errorf("negative slice = %q trunc=%v, want empty true", out.Bytes(), trunc)
	}
}

func TestChunk(t *testing.T) {
	b := Bytes("abcdefg")
	full := b.Chunk(3, false) // drop short tail
	if len(full) != 2 || full[0].String() != "abc" || full[1].String() != "def" {
		t.Errorf("chunk drop-tail = %v", chunkStrings(full))
	}
	kept := b.Chunk(3, true) // keep short tail
	if len(kept) != 3 || kept[2].String() != "g" {
		t.Errorf("chunk keep-tail = %v", chunkStrings(kept))
	}
}

func TestEvery(t *testing.T) {
	b := Bytes("abcde")
	win := b.Every(1, 2) // sliding windows of 2
	want := []string{"ab", "bc", "cd", "de"}
	if got := chunkStrings(win); !eqStrings(got, want) {
		t.Errorf("every = %v, want %v", got, want)
	}
}

func TestStrRuneAware(t *testing.T) {
	s := Str("héllo") // é is 2 bytes, 1 rune
	if s.Len() != 5 {
		t.Errorf("rune len = %d, want 5", s.Len())
	}
	g, ok := s.Get(1)
	if !ok || g.String() != "é" {
		t.Errorf("Get(1) = %q ok=%v, want é", g, ok)
	}
	out, _ := s.Slice(1, 3, 1)
	if out.String() != "él" {
		t.Errorf("slice = %q, want él", out.String())
	}
}

// ── walk / bysjq selection ────────────────────────────────────────────────

func TestWalkDiscreteAndContinuous(t *testing.T) {
	v, _ := Decode([]byte(`{"user":{"names":["ann","bob"]}}`), "json")
	got, ok, err := Walk(v, Path{"user", "names", "1"})
	if err != nil || !ok {
		t.Fatalf("walk err=%v ok=%v", err, ok)
	}
	if got.String() != "bob" {
		t.Errorf("walk = %q, want bob", got.String())
	}

	_, ok, _ = Walk(v, Path{"user", "missing"})
	if ok {
		t.Error("expected miss on absent key")
	}
}

func TestByJQ(t *testing.T) {
	v, _ := Decode([]byte(`{"items":[{"id":1},{"id":2}]}`), "json")
	got, ok, err := ByJQ(v, ".items[1].id")
	if err != nil || !ok {
		t.Fatalf("byjq err=%v ok=%v", err, ok)
	}
	if got.String() != "2" {
		t.Errorf("byjq = %q, want 2", got.String())
	}
}

// ── discrete helpers ──────────────────────────────────────────────────────

func TestTryGet(t *testing.T) {
	o := Object{"0": Str("zero"), "1": Str("one")}
	v, ok, err := TryGet[int](o, "1", strconv.Atoi)
	if err != nil || !ok || v.String() != "one" {
		t.Errorf("tryget valid = %q ok=%v err=%v", v, ok, err)
	}
	_, ok, err = TryGet[int](o, "notanumber", strconv.Atoi)
	if err == nil {
		t.Error("expected coercion error for non-numeric key")
	}
	_, ok, err = TryGet[int](o, "9", strconv.Atoi)
	if ok || err != nil {
		t.Errorf("tryget missing = ok=%v err=%v, want false nil", ok, err)
	}
}

func TestTranspose(t *testing.T) {
	v, _ := Decode([]byte(`{"a":{"b":"deep"},"x":"top"}`), "json")
	out, missing := Transpose(v,
		[]Path{{"a", "b"}, {"x"}, {"nope"}},
		[]string{"deep", "top", "gone"},
	)
	if out["deep"].String() != "deep" || out["top"].String() != "top" {
		t.Errorf("transpose = %v", out)
	}
	if len(missing) != 1 || missing[0][0] != "nope" {
		t.Errorf("missing = %v, want [nope]", missing)
	}
}

func TestFromBytesAs(t *testing.T) {
	obj, err := FromBytes[Object]([]byte(`{"k":"v"}`), "json")
	if err != nil {
		t.Fatal(err)
	}
	if obj["k"].String() != "v" {
		t.Errorf("frombytes object = %v", obj)
	}
	// wrong target type surfaces an error, not a panic
	if _, err := FromBytes[List]([]byte(`{"k":"v"}`), "json"); err == nil {
		t.Error("expected type mismatch error")
	}
}

func TestAs(t *testing.T) {
	var v Value = Bytes("hi")
	if b, ok := As[Bytes](v); !ok || string(b) != "hi" {
		t.Errorf("As[Bytes] = %q ok=%v", b, ok)
	}
	if _, ok := As[Object](v); ok {
		t.Error("As[Object] on Bytes should be false")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────

func chunkStrings(cs []Continuous) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.String()
	}
	return out
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
