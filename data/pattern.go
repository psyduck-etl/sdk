package data

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// Pattern is a composable byte transform — one decode or encode step. It is
// the machinery behind Continuous.By: a codec chain like "gzip|base64" is a
// Pattern built by composing the registered decoders.
type Pattern func([]byte) ([]byte, error)

// identity passes bytes through unchanged.
func identity(b []byte) ([]byte, error) { return b, nil }

// Chain composes patterns left to right: Chain(a, b)(x) == b(a(x)). A decode
// chain reads left to right; the matching encode chain is composed in reverse
// by the Registry.
func Chain(ps ...Pattern) Pattern {
	switch len(ps) {
	case 0:
		return identity
	case 1:
		return ps[0]
	}
	return func(b []byte) ([]byte, error) {
		var err error
		for _, p := range ps {
			if b, err = p(b); err != nil {
				return nil, err
			}
		}
		return b, nil
	}
}

// codec pairs a decoder with its inverse encoder under one name.
type codec struct {
	decode Pattern
	encode Pattern
}

// Registry resolves codec names and chain specs into Patterns. It is the
// public "give me a closure" surface of the pattern layer.
type Registry struct {
	codecs map[string]codec
}

// Patterns is the default registry, populated with the built-in codecs.
var Patterns = newRegistry()

func newRegistry() *Registry {
	r := &Registry{codecs: make(map[string]codec)}

	r.codecs["bytes"] = codec{identity, identity}

	r.codecs["base64"] = codec{
		decode: b64Decode(base64.StdEncoding),
		encode: b64Encode(base64.StdEncoding),
	}
	r.codecs["base64-url"] = codec{
		decode: b64Decode(base64.URLEncoding),
		encode: b64Encode(base64.URLEncoding),
	}
	r.codecs["hex"] = codec{
		decode: func(b []byte) ([]byte, error) {
			out := make([]byte, hex.DecodedLen(len(b)))
			n, err := hex.Decode(out, bytes.TrimSpace(b))
			return out[:n], err
		},
		encode: func(b []byte) ([]byte, error) {
			out := make([]byte, hex.EncodedLen(len(b)))
			hex.Encode(out, b)
			return out, nil
		},
	}
	r.codecs["url"] = codec{
		decode: func(b []byte) ([]byte, error) {
			s, err := url.QueryUnescape(string(b))
			return []byte(s), err
		},
		encode: func(b []byte) ([]byte, error) {
			return []byte(url.QueryEscape(string(b))), nil
		},
	}
	r.codecs["gzip"] = codec{
		decode: func(b []byte) ([]byte, error) {
			gz, err := gzip.NewReader(bytes.NewReader(b))
			if err != nil {
				return nil, fmt.Errorf("gzip: %w", err)
			}
			defer gz.Close()
			return io.ReadAll(gz)
		},
		encode: func(b []byte) ([]byte, error) {
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			if _, err := gz.Write(b); err != nil {
				return nil, fmt.Errorf("gzip: %w", err)
			}
			if err := gz.Close(); err != nil {
				return nil, fmt.Errorf("gzip: %w", err)
			}
			return buf.Bytes(), nil
		},
	}

	return r
}

func b64Decode(enc *base64.Encoding) Pattern {
	return func(b []byte) ([]byte, error) {
		out := make([]byte, enc.DecodedLen(len(b)))
		n, err := enc.Decode(out, bytes.TrimRight(b, "\n"))
		return out[:n], err
	}
}

func b64Encode(enc *base64.Encoding) Pattern {
	return func(b []byte) ([]byte, error) {
		out := make([]byte, enc.EncodedLen(len(b)))
		enc.Encode(out, b)
		return out, nil
	}
}

// Register adds or overrides a named codec.
func (r *Registry) Register(name string, decode, encode Pattern) {
	r.codecs[name] = codec{decode: decode, encode: encode}
}

// known reports whether name is a registered byte-level codec.
func (r *Registry) known(name string) bool {
	_, ok := r.codecs[name]
	return ok
}

// Decode resolves a chain spec ("gzip|base64") into a single decode Pattern,
// applied left to right.
func (r *Registry) Decode(spec string) (Pattern, error) {
	names := splitSpec(spec)
	ps := make([]Pattern, len(names))
	for i, name := range names {
		c, ok := r.codecs[name]
		if !ok {
			return nil, fmt.Errorf("unknown codec %q", name)
		}
		ps[i] = c.decode
	}
	return Chain(ps...), nil
}

// Encode resolves a chain spec into a single encode Pattern. The encoders run
// in reverse order of the spec, so "gzip|base64" encodes as base64 then gzip —
// the inverse of the decode direction.
func (r *Registry) Encode(spec string) (Pattern, error) {
	names := splitSpec(spec)
	ps := make([]Pattern, len(names))
	for i, name := range names {
		c, ok := r.codecs[name]
		if !ok {
			return nil, fmt.Errorf("unknown codec %q", name)
		}
		// reverse: last decode step is the first to undo on encode
		ps[len(names)-1-i] = c.encode
	}
	return Chain(ps...), nil
}

func splitSpec(spec string) []string {
	parts := strings.Split(spec, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"bytes"}
	}
	return out
}
