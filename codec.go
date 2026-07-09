package sdk

import "fmt"

// Codec renders bytes to and from a native Go value. A Codec is the
// pipeline-facing surface for whatever the caller means by "encoding":
// JSON, CSV, a chain like "gzip|json", or something a plugin defines
// itself. Implementations must be safe for concurrent use.
//
// Decoded values are ordinary Go data — nil, bool, a numeric type,
// string, []any, map[string]any, []byte — whichever shape the codec
// produces. Encode accepts the same universe of values plus whatever
// richer types a codec explicitly supports.
type Codec interface {
	// Decode reads b into a native Go value.
	Decode(b []byte) (any, error)
	// Encode renders v as raw bytes.
	Encode(v any) ([]byte, error)
}

// CodecFactory resolves a codec spec — a string like "json", "csv", or
// a chain like "gzip|json" — into a concrete Codec. The spec grammar
// is defined entirely by the registered factory; sdk does not interpret
// spec strings itself.
type CodecFactory func(spec string) (Codec, error)

var factory CodecFactory = func(spec string) (Codec, error) {
	return nil, fmt.Errorf("no codec factory registered; cannot resolve %q", spec)
}

// RegisterCodecs installs the process-wide CodecFactory. The host
// binary calls this once at startup with an implementation it has
// chosen (for example, psyduck's standard library registers its codec
// chain). Plugins never call this — they read codecs via GetCodec.
//
// RegisterCodecs is not safe to call concurrently with GetCodec or
// with itself; wire it up before any pipeline starts.
func RegisterCodecs(f CodecFactory) {
	if f == nil {
		return
	}
	factory = f
}

// GetCodec resolves spec via the currently registered factory. This is
// the plugin-facing entry point: a plugin that accepts an "encoding"
// config option calls GetCodec(config.Encoding) to obtain a Codec.
func GetCodec(spec string) (Codec, error) {
	return factory(spec)
}
