package sdk

import (
	"strings"
	"testing"
)

// psyConfig exercises every value shape the spec system can describe,
// with kebab-case psy tags like real plugin configs use.
type psyConfig struct {
	Connection string         `psy:"connection"`
	ChunkSize  int            `psy:"insert-chunk-size"`
	Ratio      float64        `psy:"ratio"`
	Enabled    bool           `psy:"enabled"`
	Fields     []string       `psy:"fields"`
	Counts     map[string]int `psy:"counts"`
	Nested     psyNested      `psy:"nested"`
	Untagged   string
	skipped    string //nolint:unused // proves unexported fields are ignored
}

type psyNested struct {
	Label string `psy:"label"`
	Depth int    `psy:"depth"`
}

func TestJSONBlockDecodeTagged(t *testing.T) {
	data := []byte(`{
		"connection": "root@tcp(localhost)/db",
		"insert-chunk-size": 128,
		"ratio": 0.5,
		"enabled": true,
		"fields": ["id", "name"],
		"counts": {"a": 1, "b": 2},
		"nested": {"label": "deep", "depth": 3},
		"untagged": "found-me",
		"skipped": "never"
	}`)

	block := NewJSONBlock(SourceRange{SourceName: "test.psy", StartLine: 1, StartCol: 1, EndLine: 2, EndCol: 1}, data)
	cfg := &psyConfig{}
	if err := block.Decode(cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if cfg.Connection != "root@tcp(localhost)/db" {
		t.Errorf("Connection = %q", cfg.Connection)
	}
	if cfg.ChunkSize != 128 {
		t.Errorf("ChunkSize = %d, want 128", cfg.ChunkSize)
	}
	if cfg.Ratio != 0.5 {
		t.Errorf("Ratio = %v, want 0.5", cfg.Ratio)
	}
	if !cfg.Enabled {
		t.Error("Enabled = false, want true")
	}
	if len(cfg.Fields) != 2 || cfg.Fields[0] != "id" || cfg.Fields[1] != "name" {
		t.Errorf("Fields = %v", cfg.Fields)
	}
	if cfg.Counts["a"] != 1 || cfg.Counts["b"] != 2 {
		t.Errorf("Counts = %v", cfg.Counts)
	}
	if cfg.Nested.Label != "deep" || cfg.Nested.Depth != 3 {
		t.Errorf("Nested = %+v", cfg.Nested)
	}
	if cfg.Untagged != "found-me" {
		t.Errorf("Untagged = %q, want case-insensitive field-name fallback", cfg.Untagged)
	}
	if cfg.skipped != "" {
		t.Errorf("skipped = %q, unexported fields must be ignored", cfg.skipped)
	}
}

func TestJSONBlockDecodeNullAndMissing(t *testing.T) {
	cfg := &psyConfig{Connection: "keep", ChunkSize: 7}
	block := NewJSONBlock(SourceRange{}, []byte(`{"connection": null, "ratio": 1.5}`))
	if err := block.Decode(cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.Connection != "keep" {
		t.Errorf("Connection = %q, JSON null must leave destination untouched", cfg.Connection)
	}
	if cfg.ChunkSize != 7 {
		t.Errorf("ChunkSize = %d, missing keys must leave destination untouched", cfg.ChunkSize)
	}
	if cfg.Ratio != 1.5 {
		t.Errorf("Ratio = %v, want 1.5", cfg.Ratio)
	}
}

func TestJSONBlockDecodeEmpty(t *testing.T) {
	cfg := &psyConfig{Connection: "keep"}
	if err := NewJSONBlock(SourceRange{}, nil).Decode(cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.Connection != "keep" {
		t.Errorf("Connection = %q, empty block must be a no-op", cfg.Connection)
	}
}

func TestJSONBlockDecodeTypeError(t *testing.T) {
	cfg := &psyConfig{}
	err := NewJSONBlock(SourceRange{SourceName: "bad.psy", StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 9}, []byte(`{"insert-chunk-size": "nope"}`)).Decode(cfg)
	if err == nil {
		t.Fatal("Decode: expected type error")
	}
	if !strings.Contains(err.Error(), "insert-chunk-size") {
		t.Errorf("error %q should name the offending field", err)
	}
	if !strings.Contains(err.Error(), "bad.psy:3:1-3:9") {
		t.Errorf("error %q should carry the block origin", err)
	}
}

func TestJSONBlockDecodeIntegralFloat(t *testing.T) {
	// cty renders all numbers alike, so 128.0 must decode into an int field.
	cfg := &psyConfig{}
	if err := NewJSONBlock(SourceRange{}, []byte(`{"insert-chunk-size": 128.0}`)).Decode(cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.ChunkSize != 128 {
		t.Errorf("ChunkSize = %d, want 128", cfg.ChunkSize)
	}

	if err := NewJSONBlock(SourceRange{}, []byte(`{"insert-chunk-size": 128.5}`)).Decode(&psyConfig{}); err == nil {
		t.Fatal("Decode: fractional value into int field should error")
	}
}

func TestJSONBlockDecodeNonPointer(t *testing.T) {
	if err := DecodeJSONTagged([]byte(`{}`), psyConfig{}); err == nil {
		t.Fatal("DecodeJSONTagged: non-pointer destination should error")
	}
}

func TestJSONBlockDecodeInterfaceField(t *testing.T) {
	type anyCfg struct {
		Value any `psy:"value"`
	}
	cfg := &anyCfg{}
	if err := NewJSONBlock(SourceRange{}, []byte(`{"value": {"n": 3, "f": 1.5, "l": [1, "two"]}}`)).Decode(cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	m, ok := cfg.Value.(map[string]any)
	if !ok {
		t.Fatalf("Value = %T, want map", cfg.Value)
	}
	if m["n"] != int64(3) {
		t.Errorf("n = %#v, want int64(3)", m["n"])
	}
	if m["f"] != 1.5 {
		t.Errorf("f = %#v, want 1.5", m["f"])
	}
	l, ok := m["l"].([]any)
	if !ok || len(l) != 2 || l[0] != int64(1) || l[1] != "two" {
		t.Errorf("l = %#v", m["l"])
	}
}

func TestJSONBlockEncodeRoundTrip(t *testing.T) {
	origin := SourceRange{SourceName: "loop.psy", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	block := NewJSONBlock(origin, []byte(`{"connection": "x"}`))

	raw, err := block.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	again := NewJSONBlock(block.Origin(), raw)
	cfg := &psyConfig{}
	if err := again.Decode(cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.Connection != "x" {
		t.Errorf("Connection = %q after round trip", cfg.Connection)
	}
	if again.Origin() != origin {
		t.Errorf("Origin = %v, want %v", again.Origin(), origin)
	}

	empty, err := NewJSONBlock(SourceRange{}, nil).Encode()
	if err != nil {
		t.Fatalf("Encode empty: %v", err)
	}
	if string(empty) != "{}" {
		t.Errorf("Encode empty = %q, want {}", empty)
	}
}
