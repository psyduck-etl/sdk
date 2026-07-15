package sdk

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// NewJSONBlock builds a ConfigBlock over a JSON object, as produced by
// ConfigBlock.Encode on the other side of a process boundary. Decode maps
// the object's keys onto psy:-tagged struct fields; keys without a
// matching field are ignored, and JSON nulls leave the destination field
// untouched (mirroring an absent attribute).
//
// An empty or nil data slice is a valid, empty block: Decode becomes a
// no-op and Encode returns "{}".
func NewJSONBlock(origin SourceRange, data []byte) ConfigBlock {
	return &jsonBlock{origin: origin, data: data}
}

type jsonBlock struct {
	origin SourceRange
	data   []byte
}

func (b *jsonBlock) Origin() SourceRange { return b.origin }

func (b *jsonBlock) Encode() ([]byte, error) {
	if len(b.data) == 0 {
		return []byte("{}"), nil
	}
	return b.data, nil
}

func (b *jsonBlock) Decode(dst any) error {
	if len(b.data) == 0 {
		return nil
	}
	if err := DecodeJSONTagged(b.data, dst); err != nil {
		return fmt.Errorf("%s: failed to decode options: %w", b.origin, err)
	}
	return nil
}

// DecodeJSONTagged unmarshals a JSON object into dst, matching object keys
// against psy: struct tags. Fields without a psy tag fall back to a
// case-insensitive match on the field name. dst must be a non-nil pointer.
//
// Numbers are decoded with json.Number precision and converted to the
// destination's numeric kind; JSON null leaves the destination untouched.
func DecodeJSONTagged(data []byte, dst any) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("decode destination must be a non-nil pointer, got %T", dst)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	return assignJSON(raw, rv, "")
}

// assignJSON writes the decoded JSON value v into rv. path names the
// position within the root object for error messages ("" at the root).
func assignJSON(v any, rv reflect.Value, path string) error {
	if v == nil {
		return nil // JSON null: leave the destination as-is
	}

	// Allocate through pointers.
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv = rv.Elem()
	}

	// An untyped interface{} destination takes the native value.
	if rv.Kind() == reflect.Interface && rv.NumMethod() == 0 {
		rv.Set(reflect.ValueOf(nativeJSON(v)))
		return nil
	}

	switch rv.Kind() {
	case reflect.String:
		s, ok := v.(string)
		if !ok {
			return typeError(path, "string", v)
		}
		rv.SetString(s)

	case reflect.Bool:
		b, ok := v.(bool)
		if !ok {
			return typeError(path, "bool", v)
		}
		rv.SetBool(b)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, ok := v.(json.Number)
		if !ok {
			return typeError(path, "number", v)
		}
		i, err := n.Int64()
		if err != nil {
			// Accept integral floats (cty renders all numbers alike).
			f, ferr := n.Float64()
			if ferr != nil || f != float64(int64(f)) {
				return fmt.Errorf("%s: cannot represent %s as integer", pathName(path), n)
			}
			i = int64(f)
		}
		if rv.OverflowInt(i) {
			return fmt.Errorf("%s: %d overflows %s", pathName(path), i, rv.Type())
		}
		rv.SetInt(i)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, ok := v.(json.Number)
		if !ok {
			return typeError(path, "number", v)
		}
		i, err := n.Int64()
		if err != nil || i < 0 {
			return fmt.Errorf("%s: cannot represent %s as %s", pathName(path), n, rv.Type())
		}
		u := uint64(i)
		if rv.OverflowUint(u) {
			return fmt.Errorf("%s: %d overflows %s", pathName(path), u, rv.Type())
		}
		rv.SetUint(u)

	case reflect.Float32, reflect.Float64:
		n, ok := v.(json.Number)
		if !ok {
			return typeError(path, "number", v)
		}
		f, err := n.Float64()
		if err != nil {
			return fmt.Errorf("%s: cannot represent %s as %s", pathName(path), n, rv.Type())
		}
		rv.SetFloat(f)

	case reflect.Slice:
		// []byte decodes from a base64 string, matching encoding/json.
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			s, ok := v.(string)
			if !ok {
				return typeError(path, "base64 string", v)
			}
			raw, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				return fmt.Errorf("%s: %w", pathName(path), err)
			}
			rv.SetBytes(raw)
			return nil
		}
		l, ok := v.([]any)
		if !ok {
			return typeError(path, "list", v)
		}
		out := reflect.MakeSlice(rv.Type(), len(l), len(l))
		for i, e := range l {
			if err := assignJSON(e, out.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		rv.Set(out)

	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("%s: unsupported map key type %s", pathName(path), rv.Type().Key())
		}
		m, ok := v.(map[string]any)
		if !ok {
			return typeError(path, "map", v)
		}
		out := reflect.MakeMapWithSize(rv.Type(), len(m))
		for k, e := range m {
			ev := reflect.New(rv.Type().Elem()).Elem()
			if err := assignJSON(e, ev, path+"."+k); err != nil {
				return err
			}
			out.SetMapIndex(reflect.ValueOf(k).Convert(rv.Type().Key()), ev)
		}
		rv.Set(out)

	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok {
			return typeError(path, "object", v)
		}
		t := rv.Type()
		for i := range t.NumField() {
			field := t.Field(i)
			tag := field.Tag.Get("psy")
			if tag == "-" {
				continue
			}
			// Embedded structs without their own psy tag flatten, mirroring
			// Go field promotion: their fields decode from this same object.
			// The embedded type itself may be unexported (the usual shape for
			// shared config fragments) — its promoted exported fields are
			// still settable through it.
			if field.Anonymous && tag == "" {
				ft := field.Type
				if ft.Kind() == reflect.Struct ||
					(ft.Kind() == reflect.Pointer && ft.Elem().Kind() == reflect.Struct && field.IsExported()) {
					if err := assignJSON(v, rv.Field(i), path); err != nil {
						return err
					}
					continue
				}
			}
			if !field.IsExported() {
				continue
			}
			e, ok := lookupField(m, tag, field.Name)
			if !ok {
				continue
			}
			if err := assignJSON(e, rv.Field(i), path+"."+fieldLabel(tag, field.Name)); err != nil {
				return err
			}
		}

	default:
		return fmt.Errorf("%s: unsupported destination type %s", pathName(path), rv.Type())
	}
	return nil
}

// lookupField finds the JSON object entry for a struct field: an exact
// match on the psy tag when present, otherwise a case-insensitive match on
// the field name.
func lookupField(m map[string]any, tag, name string) (any, bool) {
	if tag != "" {
		e, ok := m[tag]
		return e, ok
	}
	if e, ok := m[name]; ok {
		return e, true
	}
	for k, e := range m {
		if strings.EqualFold(k, name) {
			return e, true
		}
	}
	return nil, false
}

func fieldLabel(tag, name string) string {
	if tag != "" {
		return tag
	}
	return name
}

// nativeJSON converts json.Number leaves into int64 (when integral) or
// float64 so interface{} destinations hold ordinary Go numbers.
func nativeJSON(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = nativeJSON(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = nativeJSON(e)
		}
		return out
	default:
		return v
	}
}

func typeError(path, want string, got any) error {
	return fmt.Errorf("%s: expected %s, got %T", pathName(path), want, got)
}

func pathName(path string) string {
	if path == "" {
		return "(root)"
	}
	return strings.TrimPrefix(path, ".")
}
