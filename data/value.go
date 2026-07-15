// Package data is the abstract data model that the psyduck standard library
// is built on. Every message flowing through a pipeline is raw []byte, but
// most operations want to treat those bytes as structured data: a string, a
// JSON object, a list, a gzip blob, and so on.
//
// The model has two interfaces:
//
//   - Continuous — array-like data, meaningfully referenced only by index or
//     by linear pattern matching ([]byte, string, []rune, []Value, and
//     encoded byte forms like base64/gzip).
//   - Discrete — struct-like data, referenced by key (JSON object, HCL body).
//
// Both satisfy Value. Because a pipeline decodes values from bytes at runtime
// — the concrete kind is rarely known statically — the interfaces are
// non-generic and carry a Kind discriminator. Generics live only at the typed
// boundaries (FromBytes, As, the ~[]E slice helpers, Transpose).
package data

import "fmt"

// Kind discriminates the concrete shape behind a Value. It lets pipeline code
// dispatch on continuous-vs-discrete without a type switch on every op.
type Kind int

const (
	KindBytes  Kind = iota // raw []byte
	KindStr                // UTF-8 (or other) text
	KindRunes              // []rune
	KindList               // ordered []Value (continuous, array-like)
	KindObject             // keyed map[string]Value (discrete, struct-like)
	KindLit                // atomic JSON scalar leaf (number, bool, null)
)

// String names a Kind for diagnostics.
func (k Kind) String() string {
	switch k {
	case KindBytes:
		return "bytes"
	case KindStr:
		return "string"
	case KindRunes:
		return "runes"
	case KindList:
		return "list"
	case KindObject:
		return "object"
	case KindLit:
		return "lit"
	default:
		return fmt.Sprintf("kind(%d)", int(k))
	}
}

// Continuous returns true for kinds that are array-like (referenced by index
// or linear pattern): bytes, string, runes, list. Objects (discrete) and Lits
// (atomic leaves) are not.
func (k Kind) Continuous() bool { return k != KindObject && k != KindLit }

// Value is the shared surface of the data model. Everything is a Value.
type Value interface {
	// Kind reports the concrete shape, so callers can dispatch without a
	// type switch.
	Kind() Kind
	// String renders the value as text.
	String() string
	// Bytes renders the value as raw bytes — the canonical encoding for
	// handing the value back to the pipeline as a []byte message.
	Bytes() []byte
}

// As recovers a concrete Value type after a boxed operation. Go forbids
// generic methods, so this is a free function rather than a Value method.
//
//	b, ok := data.As[data.Bytes](v)
func As[T Value](v Value) (T, bool) {
	t, ok := v.(T)
	return t, ok
}

// FromBytes decodes raw bytes into a concrete Value type using a named codec
// or parser ("bytes", "utf-8", "ascii", "latin1", "json", "csv", "hcl", ...).
// The type parameter appears only in the result, so callers instantiate it
// explicitly:
//
//	obj, err := data.FromBytes[data.Object](b, "json")
func FromBytes[T Value](b []byte, as string) (T, error) {
	v, err := Decode(b, as)
	if err != nil {
		var zero T
		return zero, err
	}

	t, ok := v.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("decoded %s as %q but wanted %T", v.Kind(), as, t)
	}
	return t, nil
}
