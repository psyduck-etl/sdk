package data

import (
	"github.com/psyduck-etl/sdk"
)

// InputCodec is an embeddable config fragment for resources that need to decode
// incoming record bytes. Embed it in your resource config struct, call Bind()
// after parsing, and use Decode() to unmarshal records.
//
// Example:
//
//	type MyConsumerConfig struct {
//	    Table string `psy:"table"`
//	    InputCodec
//	}
//
//	// In your ProvideConsumer:
//	config := new(MyConsumerConfig)
//	if err := parse(config); err != nil { return nil, err }
//	if err := config.Bind(); err != nil { return nil, err }
//	// Now config.Decode(bytes) is ready to use
//
// The Accept field is resolved via sdk.GetCodec() at bind time; the spec is
// passed through verbatim, so any normalization is up to the host's factory.
// The Decode method may return any decoded native type; wrap
// it with custom validation if needed (e.g., type-checking for objects).
type InputCodec struct {
	Accept string `psy:"accept"`
	codec  sdk.Codec
}

// Bind resolves the Accept codec via the sdk-registered factory. The host
// installs a codec factory at startup; tests can register stubs in TestMain.
// Returns an error if the codec name is unknown.
func (c *InputCodec) Bind() error {
	var err error
	c.codec, err = sdk.GetCodec(c.Accept)
	return err
}

// Decode decodes one record via the bound Accept codec. Returns any native
// type the codec produces (usually map[string]any for structured codecs,
// string for terminal ones). Callers should validate the type if they need
// a specific shape (e.g., requiring an object for database operations).
func (c *InputCodec) Decode(data []byte) (any, error) {
	return c.codec.Decode(data)
}

// DecodeValue decodes one record straight into the sdk's richer typed Value
// representation, instead of the lossy native `any` that Decode produces.
// Native flattens every decoded shape down to nil/bool/number/string/
// []any/map[string]any, so a caller that wants to hand the result to
// another Value-aware transformer (data.Walk, data.ByJQ, a codecTransformer
// op, ...) would otherwise have to re-derive a Value from that native data.
// DecodeValue skips that trip: it decodes bytes to a Value once, and the
// concrete Kind (list vs object vs string vs bytes) is preserved exactly.
//
// DecodeValue resolves the Accept spec through the data package's own codec
// chain (the same grammar as Decode/Encode) rather than through the bound
// sdk.Codec, so — unlike Decode — it does not require Bind() first.
func (c *InputCodec) DecodeValue(data []byte) (Value, error) {
	return Decode(data, c.Accept)
}

// Sparse reports whether the Accept codec carries bare terminal references
// (strings, scalars) rather than structured objects. True for "string" codec,
// false for discrete codecs like json/yaml/csv.
func (c *InputCodec) Sparse() bool {
	return c.Accept == "string"
}

// Spec returns the reusable spec definition for the Accept field, suitable for
// embedding in resource specs via append():
//
//	var mySpec = []*sdk.Spec{
//	    ...,
//	    InputCodec.Spec()[0],
//	}
func (c *InputCodec) Spec() []*sdk.Spec {
	return []*sdk.Spec{
		{
			Name:        "accept",
			Description: "Codec spec used to decode record bytes. Resolved via the host's registered codec factory — psyduck's stdlib accepts names like \"json\" and \"csv\" as well as chains like \"gzip|json\"",
			Required:    false,
			Type:        sdk.TypeString,
			Default:     "json",
		},
	}
}

// OutputCodec is an embeddable config fragment for resources that need to encode
// outgoing records to bytes. Embed it in your resource config struct, call Bind()
// after parsing, and use Encode() to marshal records.
//
// Example:
//
//	type MyProducerConfig struct {
//	    Query string `psy:"query"`
//	    OutputCodec
//	}
//
//	// In your ProvideProducer:
//	config := new(MyProducerConfig)
//	if err := parse(config); err != nil { return nil, err }
//	if err := config.Bind(); err != nil { return nil, err }
//	// Now config.Encode(record) is ready to use
//
// The Emit field is resolved via sdk.GetCodec() at bind time; the spec is
// passed through verbatim, so any normalization is up to the host's factory.
// The Encode method accepts any native type the codec
// expects (usually map[string]any for structured codecs, string for terminals).
type OutputCodec struct {
	Emit  string `psy:"emit"`
	codec sdk.Codec
}

// Bind resolves the Emit codec via the sdk-registered factory. The host
// installs a codec factory at startup; tests can register stubs in TestMain.
// Returns an error if the codec name is unknown.
func (c *OutputCodec) Bind() error {
	var err error
	c.codec, err = sdk.GetCodec(c.Emit)
	return err
}

// Encode encodes one record via the bound Emit codec. Expects the native type
// the codec accepts (usually map[string]any for structured codecs, string for
// terminals). Returns the codec's byte output.
func (c *OutputCodec) Encode(v any) ([]byte, error) {
	return c.codec.Encode(v)
}

// EncodeValue is the Value-typed counterpart to Encode: it renders a Value
// straight to bytes via the Emit spec, without lowering it to native `any`
// first. Pairing DecodeValue with EncodeValue lets a chain of Value-aware
// operations (walk, transpose, jq, ...) pass a single Value between them and
// hit the codec chain only at the two ends, instead of re-serializing to
// native data (and back) at every step.
//
// EncodeValue resolves the Emit spec through the data package's own codec
// chain rather than through the bound sdk.Codec, so — unlike Encode — it
// does not require Bind() first.
func (c *OutputCodec) EncodeValue(v Value) ([]byte, error) {
	return Encode(v, c.Emit)
}

// Sparse reports whether the Emit codec carries bare terminal references
// (strings, scalars) rather than structured objects. True for "string" codec,
// false for discrete codecs like json/yaml/csv.
func (c *OutputCodec) Sparse() bool {
	return c.Emit == "string"
}

// Spec returns the reusable spec definition for the Emit field, suitable for
// embedding in resource specs via append():
//
//	var mySpec = []*sdk.Spec{
//	    ...,
//	    OutputCodec.Spec()[0],
//	}
func (c *OutputCodec) Spec() []*sdk.Spec {
	return []*sdk.Spec{
		{
			Name:        "emit",
			Description: "Codec spec used to encode record bytes. Resolved via the host's registered codec factory — psyduck's stdlib accepts names like \"json\" and \"csv\" as well as chains like \"gzip|json\"",
			Required:    false,
			Type:        sdk.TypeString,
			Default:     "json",
		},
	}
}

// IsTerminalRef reports whether spec names the "string" codec — the one that
// carries bare scalar references (IDs, names) rather than structured objects.
// Transformers branching on terminal vs structured data can use this to decide
// whether to emit the full object or just its reference.
func IsTerminalRef(spec string) bool {
	return spec == "string"
}
