package sdk

import "fmt"

// AcceptConfig is an embeddable config fragment for resources that decode input.
// Embed it in your resource config struct, call Bind() after parsing, then use
// Decode() to decode records. The Accept field is required and must match a
// codec name registered with the host (e.g., "json", "csv", "string").
type AcceptConfig struct {
	Accept string `psy:"accept"`
	codec  Codec
}

// Bind resolves the accept codec via the sdk-registered factory. Returns an
// error if the codec spec is unknown or empty. Must be called once after parsing.
func (c *AcceptConfig) Bind() error {
	if c.Accept == "" {
		return fmt.Errorf("accept: codec required, got empty string")
	}
	var err error
	c.codec, err = GetCodec(c.Accept)
	return err
}

// Decode decodes one record via the bound accept codec.
func (c *AcceptConfig) Decode(data []byte) (any, error) {
	if c.codec == nil {
		return nil, fmt.Errorf("accept: Decode called before Bind()")
	}
	return c.codec.Decode(data)
}

// EmitConfig is an embeddable config fragment for resources that encode output.
// Embed it in your resource config struct, call Bind() after parsing, then use
// Encode() to encode records. The Emit field is required and must match a
// codec name registered with the host.
type EmitConfig struct {
	Emit  string `psy:"emit"`
	codec Codec
}

// Bind resolves the emit codec via the sdk-registered factory. Returns an
// error if the codec spec is unknown or empty. Must be called once after parsing.
func (c *EmitConfig) Bind() error {
	if c.Emit == "" {
		return fmt.Errorf("emit: codec required, got empty string")
	}
	var err error
	c.codec, err = GetCodec(c.Emit)
	return err
}

// Encode encodes one record via the bound emit codec.
func (c *EmitConfig) Encode(v any) ([]byte, error) {
	if c.codec == nil {
		return nil, fmt.Errorf("emit: Encode called before Bind()")
	}
	return c.codec.Encode(v)
}

// CodecConfig is a composite for resources that both accept and emit.
// Embed it in your config, call Bind() after parsing, then use
// Decode()/Encode(). Accept and Emit can differ (e.g., accept "csv",
// emit "json" for a CSV-to-JSON transformer).
type CodecConfig struct {
	AcceptConfig
	EmitConfig
}

// Bind resolves both accept and emit codecs. Returns the first error encountered.
func (c *CodecConfig) Bind() error {
	if err := c.AcceptConfig.Bind(); err != nil {
		return err
	}
	return c.EmitConfig.Bind()
}

// IsTerminalRef reports whether spec names the "string" codec — the one that
// carries bare scalar references (IDs, names) rather than structured objects.
func IsTerminalRef(spec string) bool {
	return spec == "string"
}
