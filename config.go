package sdk

import "fmt"

// ConfigBlock is a format-agnostic handle onto a single configuration
// block. Hosts implement it (typically over an hcl.Body); plugins consume
// it via Decode.
//
// Decode's signature intentionally matches Parser so that a bound
// block.Decode value may be passed directly to Provider closures.
type ConfigBlock interface {
	Origin() SourceRange
	Decode(dst any) error
}

// SourceRange identifies a span of source text in a config file. It is
// used for diagnostic messages. The zero value is meaningful and renders
// as "<unknown>".
type SourceRange struct {
	SourceName string
	StartLine  int
	StartCol   int
	EndLine    int
	EndCol     int
}

// String renders the range like "pipeline.psy:12:3-14:1". A zero-valued
// SourceRange renders as "<unknown>".
func (r SourceRange) String() string {
	if r == (SourceRange{}) {
		return "<unknown>"
	}
	return fmt.Sprintf("%s:%d:%d-%d:%d",
		r.SourceName, r.StartLine, r.StartCol, r.EndLine, r.EndCol)
}

// BlockMeta holds host-owned attributes that appear on every resource
// block. Hosts decode this out-of-band, before calling Plugin.Bind, so
// plugins never observe these fields.
type BlockMeta struct {
	PerMinute int `psy:"per-minute"`
	StopAfter int `psy:"stop-after"`
}
