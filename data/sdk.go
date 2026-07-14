package data

import "github.com/psyduck-etl/sdk"

// Codec adapts the stdlib codec chain (Decode/Encode over Value) into the
// spec/native surface sdk.Codec exposes. Hosts install it once with
// sdk.RegisterCodecs, after which any plugin that calls sdk.GetCodec
// gets the same chain resolver — "json", "csv", "gzip|json", etc.
//
// This is the only place the stdlib data model meets the sdk codec
// surface: Value stays here, native Go data (map[string]any, []any,
// scalars) crosses the boundary.
func Codec(spec string) (sdk.Codec, error) {
	return chainCodec{spec: spec}, nil
}

// chainCodec is stateless. Decode/Encode re-run the chain each call, so
// the same instance is safe to share across goroutines.
type chainCodec struct{ spec string }

func (c chainCodec) Decode(b []byte) (any, error) {
	v, err := Decode(b, c.spec)
	if err != nil {
		return nil, err
	}
	return Native(v), nil
}

func (c chainCodec) Encode(v any) ([]byte, error) {
	return Encode(fromNative(v), c.spec)
}
