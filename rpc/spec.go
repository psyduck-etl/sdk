package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

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
		if _, err := dec.Token(); err != io.EOF {
			return nil, fmt.Errorf("spec %s: decode default: trailing data after value", p.Name)
		}
		out.Default = nativeDefault(v, out)
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

// nativeDefault rewrites json.Number leaves into int64/float64 so a
// round-tripped default is an ordinary Go value. It walks the value tree
// alongside the spec tree, so a leaf described by the spec converts to its
// declared kind — JSON cannot tell 2.0 from 2, but TypeFloat can. Leaves
// the spec does not describe (unmatched object keys, missing ElemType,
// unknown Type) fall back to nativeNumber's integral-first heuristic.
func nativeDefault(v any, s *sdk.Spec) any {
	switch t := v.(type) {
	case json.Number:
		return nativeNumber(t, s)
	case []any:
		var elem *sdk.Spec
		if s != nil {
			elem = s.ElemType
		}
		for i, e := range t {
			t[i] = nativeDefault(e, elem)
		}
		return t
	case map[string]any:
		switch {
		case s != nil && s.Type == sdk.TypeObject:
			fields := make(map[string]*sdk.Spec, len(s.Fields))
			for _, f := range s.Fields {
				fields[f.Name] = f
			}
			for k, e := range t {
				t[k] = nativeDefault(e, fields[k])
			}
		case s != nil && s.ElemType != nil:
			for k, e := range t {
				t[k] = nativeDefault(e, s.ElemType)
			}
		default:
			for k, e := range t {
				t[k] = nativeDefault(e, nil)
			}
		}
		return t
	default:
		return v
	}
}

// nativeNumber converts one number leaf, honoring the spec's declared type
// when there is one and it fits; otherwise integral-first, degrading to
// float64 and finally the digit string (overflow-safe).
func nativeNumber(n json.Number, s *sdk.Spec) any {
	if s != nil {
		switch s.Type {
		case sdk.TypeFloat:
			if f, err := n.Float64(); err == nil {
				return f
			}
		case sdk.TypeInt:
			if i, err := n.Int64(); err == nil {
				return i
			}
		}
	}
	if i, err := n.Int64(); err == nil {
		return i
	}
	if f, err := n.Float64(); err == nil {
		return f
	}
	return n.String()
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
