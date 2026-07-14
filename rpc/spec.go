package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/psyduck-etl/sdk"
	"github.com/psyduck-etl/sdk/proto"
)

// specToProto renders an sdk.Spec tree onto the wire. Defaults cross
// JSON-encoded because Spec.Default is an open Go value.
func specToProto(s *sdk.Spec) (*proto.Spec, error) {
	if s == nil {
		return nil, nil
	}

	out := &proto.Spec{
		Name:        s.Name,
		Description: s.Description,
		Required:    s.Required,
		Type:        uint32(s.Type),
	}

	var err error
	if out.ElemType, err = specToProto(s.ElemType); err != nil {
		return nil, err
	}
	if out.Fields, err = specsToProto(s.Fields); err != nil {
		return nil, err
	}

	if s.Default != nil {
		raw, err := json.Marshal(s.Default)
		if err != nil {
			return nil, fmt.Errorf("spec %s: encode default: %w", s.Name, err)
		}
		out.DefaultJson = raw
	}
	return out, nil
}

func specsToProto(specs []*sdk.Spec) ([]*proto.Spec, error) {
	if specs == nil {
		return nil, nil
	}
	out := make([]*proto.Spec, len(specs))
	for i, s := range specs {
		p, err := specToProto(s)
		if err != nil {
			return nil, err
		}
		out[i] = p
	}
	return out, nil
}

// specFromProto rebuilds an sdk.Spec tree. Numeric defaults come back as
// int64 (when integral) or float64 — ordinary Go values the host's config
// format converts with the same machinery it uses for author-written
// defaults.
func specFromProto(p *proto.Spec) (*sdk.Spec, error) {
	if p == nil {
		return nil, nil
	}

	out := &sdk.Spec{
		Name:        p.Name,
		Description: p.Description,
		Required:    p.Required,
		Type:        sdk.SpecType(p.Type),
	}

	var err error
	if out.ElemType, err = specFromProto(p.ElemType); err != nil {
		return nil, err
	}
	if out.Fields, err = specsFromProto(p.Fields); err != nil {
		return nil, err
	}

	if len(p.DefaultJson) > 0 {
		dec := json.NewDecoder(bytes.NewReader(p.DefaultJson))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			return nil, fmt.Errorf("spec %s: decode default: %w", p.Name, err)
		}
		out.Default = nativeNumbers(v)
	}
	return out, nil
}

func specsFromProto(specs []*proto.Spec) ([]*sdk.Spec, error) {
	if specs == nil {
		return nil, nil
	}
	out := make([]*sdk.Spec, len(specs))
	for i, p := range specs {
		s, err := specFromProto(p)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

// nativeNumbers rewrites json.Number leaves into int64/float64 so a
// round-tripped default is an ordinary Go value.
func nativeNumbers(v any) any {
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
		for i, e := range t {
			t[i] = nativeNumbers(e)
		}
		return t
	case map[string]any:
		for k, e := range t {
			t[k] = nativeNumbers(e)
		}
		return t
	default:
		return v
	}
}

func descriptorsToProto(descs []sdk.ResourceDescriptor) ([]*proto.ResourceDescriptor, error) {
	out := make([]*proto.ResourceDescriptor, len(descs))
	for i, d := range descs {
		spec, err := specsToProto(d.Spec)
		if err != nil {
			return nil, fmt.Errorf("resource %s: %w", d.Name, err)
		}
		out[i] = &proto.ResourceDescriptor{Name: d.Name, Kinds: uint64(d.Kinds), Spec: spec}
	}
	return out, nil
}

func descriptorsFromProto(descs []*proto.ResourceDescriptor) ([]sdk.ResourceDescriptor, error) {
	out := make([]sdk.ResourceDescriptor, len(descs))
	for i, d := range descs {
		spec, err := specsFromProto(d.Spec)
		if err != nil {
			return nil, fmt.Errorf("resource %s: %w", d.Name, err)
		}
		out[i] = sdk.ResourceDescriptor{Name: d.Name, Kinds: sdk.Kind(d.Kinds), Spec: spec}
	}
	return out, nil
}

func rangeToProto(r sdk.SourceRange) *proto.SourceRange {
	return &proto.SourceRange{
		SourceName: r.SourceName,
		StartLine:  int64(r.StartLine),
		StartCol:   int64(r.StartCol),
		EndLine:    int64(r.EndLine),
		EndCol:     int64(r.EndCol),
	}
}

func rangeFromProto(r *proto.SourceRange) sdk.SourceRange {
	if r == nil {
		return sdk.SourceRange{}
	}
	return sdk.SourceRange{
		SourceName: r.SourceName,
		StartLine:  int(r.StartLine),
		StartCol:   int(r.StartCol),
		EndLine:    int(r.EndLine),
		EndCol:     int(r.EndCol),
	}
}
