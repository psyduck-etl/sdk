package data

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// ── native <-> Value ───────────────────────────────────────────────────────

// fromNative builds a Value tree from decoded native Go data (the output of
// json.Unmarshal with UseNumber).
func fromNative(n any) Value {
	switch t := n.(type) {
	case nil:
		return Lit{nil}
	case bool:
		return Lit{t}
	case float64, json.Number, int, int64:
		return Lit{t}
	case string:
		return Str(t)
	case []any:
		l := make(List, len(t))
		for i, e := range t {
			l[i] = fromNative(e)
		}
		return l
	case map[string]any:
		o := make(Object, len(t))
		for k, e := range t {
			o[k] = fromNative(e)
		}
		return o
	case Value:
		return t
	default:
		return Lit{t}
	}
}

// Native lowers a Value tree to plain Go data (map[string]any, []any, string,
// float64, bool, nil) — the shape json.Marshal, a Go text/template, or an fmt
// verb expects.
func Native(v Value) any {
	switch t := v.(type) {
	case Lit:
		return t.V
	case Str:
		return string(t)
	case Runes:
		return string(t)
	case Bytes:
		return string(t)
	case List:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = Native(e)
		}
		return out
	case Object:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = Native(e)
		}
		return out
	case Value:
		return t.String()
	default:
		return v
	}
}

// marshalJSON renders a Value tree as compact JSON.
func marshalJSON(v Value) []byte {
	b, err := json.Marshal(Native(v))
	if err != nil {
		return []byte(sprint(Native(v)))
	}
	return b
}

// ── decode: bytes -> Value ────────────────────────────────────────────────

// terminalDecoders turn raw bytes into a structured/string Value. A terminal
// decoder, if present in a chain spec, must be the last element.
var terminalDecoders = map[string]func([]byte) (Value, error){
	"utf-8":       decodeUTF8,
	"ascii":       decodeASCII,
	"latin1":      decodeLatin1,
	"runes":       func(b []byte) (Value, error) { return Runes([]rune(string(b))), nil },
	"json":        decodeJSON,
	"json-pretty": decodeJSON,
	"csv":         decodeCSV,
}

func isTerminal(name string) bool {
	_, ok := terminalDecoders[name]
	return ok
}

// Decode resolves a chain spec ("gzip|json", "utf-8", "base64") and decodes
// bytes into a Value. Byte-level codecs (from the Pattern registry) apply left
// to right; a terminal decoder, if any, must come last and produces the final
// structured/string Value. A spec with no terminal yields Bytes. FromBytes
// wraps this with a concrete-type assertion.
func Decode(b []byte, as string) (Value, error) {
	names := splitSpec(as)
	for i, name := range names {
		if fn, ok := terminalDecoders[name]; ok {
			if i != len(names)-1 {
				return nil, fmt.Errorf("decoder %q must be the last step in %q", name, as)
			}
			return fn(b)
		}
		c, ok := Patterns.codecs[name]
		if !ok {
			return nil, fmt.Errorf("unknown decoder %q", name)
		}
		out, err := c.decode(b)
		if err != nil {
			return nil, err
		}
		b = out
	}
	return Bytes(b), nil
}

// Encode is the inverse of Decode: it renders a Value into the target encoding.
// The terminal encoder (if the spec ends in one) runs first to produce bytes,
// then the byte codecs apply in reverse order.
func Encode(v Value, as string) ([]byte, error) {
	names := splitSpec(as)
	byteEnd := len(names)
	term := ""
	if n := len(names); n > 0 && isTerminal(names[n-1]) {
		term, byteEnd = names[n-1], n-1
	}

	b, err := terminalEncode(v, term)
	if err != nil {
		return nil, err
	}
	for i := byteEnd - 1; i >= 0; i-- {
		c, ok := Patterns.codecs[names[i]]
		if !ok {
			return nil, fmt.Errorf("unknown encoder %q", names[i])
		}
		if b, err = c.encode(b); err != nil {
			return nil, err
		}
	}
	return b, nil
}

func terminalEncode(v Value, term string) ([]byte, error) {
	switch term {
	case "", "bytes":
		return v.Bytes(), nil
	case "json":
		return marshalJSON(v), nil
	case "json-pretty":
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		if err := enc.Encode(Native(v)); err != nil {
			return nil, err
		}
		return bytes.TrimRight(buf.Bytes(), "\n"), nil
	case "utf-8", "ascii", "latin1", "runes":
		return []byte(v.String()), nil
	case "csv":
		return encodeCSV(v)
	default:
		return nil, fmt.Errorf("unknown encoder %q", term)
	}
}

// ── terminal decoder impls ────────────────────────────────────────────────

func decodeUTF8(b []byte) (Value, error) {
	if !utf8.Valid(b) {
		return nil, fmt.Errorf("invalid utf-8 input")
	}
	return Str(b), nil
}

func decodeASCII(b []byte) (Value, error) {
	for i, c := range b {
		if c > 127 {
			return nil, fmt.Errorf("invalid ascii byte 0x%02x at offset %d", c, i)
		}
	}
	return Str(b), nil
}

func decodeLatin1(b []byte) (Value, error) {
	rs := make([]rune, len(b))
	for i, c := range b {
		rs[i] = rune(c)
	}
	return Str(rs), nil
}

func decodeJSON(b []byte) (Value, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var n any
	if err := dec.Decode(&n); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}
	return fromNative(n), nil
}

// ── csv ───────────────────────────────────────────────────────────────────

// decodeCSV parses CSV bytes. A single record decodes to a List of Str fields;
// multiple records decode to a List of such Lists.
func decodeCSV(b []byte) (Value, error) {
	r := csv.NewReader(bytes.NewReader(b))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv: %w", err)
	}
	rows := make(List, len(records))
	for i, rec := range records {
		fields := make(List, len(rec))
		for j, f := range rec {
			fields[j] = Str(f)
		}
		rows[i] = fields
	}
	if len(rows) == 1 {
		return rows[0], nil
	}
	return rows, nil
}

// encodeCSV renders a List of fields as one CSV line, or a List of Lists as
// multiple lines.
func encodeCSV(v Value) ([]byte, error) {
	l, ok := v.(List)
	if !ok {
		return nil, fmt.Errorf("csv: want a list, got %s", v.Kind())
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	rows := [][]string{}
	if len(l) > 0 {
		if _, nested := l[0].(List); nested {
			for _, row := range l {
				rows = append(rows, csvFields(row))
			}
		} else {
			rows = append(rows, csvFields(l))
		}
	}
	if err := w.WriteAll(rows); err != nil {
		return nil, fmt.Errorf("csv: %w", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func csvFields(v Value) []string {
	l, ok := v.(List)
	if !ok {
		return []string{v.String()}
	}
	out := make([]string, len(l))
	for i, f := range l {
		out[i] = f.String()
	}
	return out
}
