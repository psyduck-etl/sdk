package data

import "testing"

// ── csv ──────────────────────────────────────────────────────────────────

func TestDecodeCSVSingleRecord(t *testing.T) {
	// A single record decodes to a flat List of fields, not a List of one
	// List — this is the len(rows)==1 special case in decodeCSV.
	v, err := Decode([]byte("a,b,c"), "csv")
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	l, ok := v.(List)
	if !ok || len(l) != 3 {
		t.Fatalf("want 3-field List, got %T %v", v, v)
	}
	if l[0].String() != "a" || l[1].String() != "b" || l[2].String() != "c" {
		t.Errorf("single record = %v, want [a b c]", l)
	}
}

func TestDecodeCSVMultiRecord(t *testing.T) {
	// Multiple records stay nested: a List of row-Lists.
	v, err := Decode([]byte("a,b\nc,d\n"), "csv")
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	rows, ok := v.(List)
	if !ok || len(rows) != 2 {
		t.Fatalf("want 2-row List, got %T %v", v, v)
	}
	row0, ok := rows[0].(List)
	if !ok || len(row0) != 2 || row0[0].String() != "a" || row0[1].String() != "b" {
		t.Errorf("row 0 = %v, want [a b]", row0)
	}
}

func TestDecodeCSVRaggedRows(t *testing.T) {
	// decodeCSV sets FieldsPerRecord = -1 deliberately: rows are allowed to
	// have different field counts.
	v, err := Decode([]byte("a,b,c\nd,e\n"), "csv")
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	rows, ok := v.(List)
	if !ok || len(rows) != 2 {
		t.Fatalf("want 2 rows, got %T %v", v, v)
	}
	row0, _ := rows[0].(List)
	row1, _ := rows[1].(List)
	if len(row0) != 3 || len(row1) != 2 {
		t.Errorf("ragged row lengths = %d,%d, want 3,2", len(row0), len(row1))
	}
}

func TestDecodeCSVMalformed(t *testing.T) {
	if _, err := Decode([]byte(`"unterminated`), "csv"); err == nil {
		t.Error("expected csv decode error for an unterminated quoted field")
	}
}

func TestEncodeCSVSingleRow(t *testing.T) {
	out, err := Encode(List{Str("a"), Str("b")}, "csv")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if string(out) != "a,b" {
		t.Errorf("encode = %q, want %q", out, "a,b")
	}
}

func TestEncodeCSVMultiRow(t *testing.T) {
	out, err := Encode(List{List{Str("a"), Str("b")}, List{Str("c"), Str("d")}}, "csv")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if string(out) != "a,b\nc,d" {
		t.Errorf("encode = %q, want %q", out, "a,b\nc,d")
	}
}

func TestEncodeCSVEmpty(t *testing.T) {
	// len(l) == 0 must short-circuit before indexing l[0].
	out, err := Encode(List{}, "csv")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("encode of empty list = %q, want empty", out)
	}
}

func TestEncodeCSVRequiresList(t *testing.T) {
	if _, err := Encode(Str("not a list"), "csv"); err == nil {
		t.Error("expected error encoding a non-List as csv")
	}
}

// ── json-pretty ──────────────────────────────────────────────────────────

func TestEncodeJSONPretty(t *testing.T) {
	v, err := Decode([]byte(`{"a":1}`), "json")
	if err != nil {
		t.Fatalf("seed Decode: %v", err)
	}
	out, err := Encode(v, "json-pretty")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	want := "{\n  \"a\": 1\n}"
	if string(out) != want {
		t.Errorf("json-pretty = %q, want %q", out, want)
	}
	if _, err := Decode(out, "json"); err != nil {
		t.Errorf("re-decode of pretty output failed: %v", err)
	}
}

// ── ascii boundary ───────────────────────────────────────────────────────

func TestDecodeASCIIAcceptsDEL(t *testing.T) {
	// 127 (DEL) is the top of the 7-bit ASCII range and must be accepted,
	// not just bytes strictly below it.
	if _, err := Decode([]byte{'a', 127}, "ascii"); err != nil {
		t.Errorf("ascii should accept byte 127 (DEL): %v", err)
	}
}

// codec.go's Encode has an `n > 0 && isTerminal(...)` guard; splitSpec never
// returns an empty slice (it falls back to []string{"bytes"}), so n is
// always >= 1 and the n == 0 case is unreachable from any caller.
