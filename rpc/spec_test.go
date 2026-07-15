package rpc

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/psyduck-etl/sdk"
)

// roundTrip pushes a spec through specToProto → specFromProto.
func roundTrip(t *testing.T, in *sdk.Spec) *sdk.Spec {
	t.Helper()
	p, err := specToProto(in)
	if err != nil {
		t.Fatalf("specToProto: %v", err)
	}
	out, err := specFromProto(p)
	if err != nil {
		t.Fatalf("specFromProto: %v", err)
	}
	return out
}

func TestSpecRoundTripStructure(t *testing.T) {
	in := &sdk.Spec{
		Name:        "outer",
		Description: "an object",
		Required:    true,
		Type:        sdk.TypeObject,
		Fields: []*sdk.Spec{
			{
				Name: "tags",
				Type: sdk.TypeList,
				ElemType: &sdk.Spec{
					Name: "tag",
					Type: sdk.TypeString,
				},
			},
			{
				Name: "nested",
				Type: sdk.TypeObject,
				Fields: []*sdk.Spec{
					{
						Name: "deep",
						Type: sdk.TypeList,
						ElemType: &sdk.Spec{
							Name: "deeper",
							Type: sdk.TypeMap,
							ElemType: &sdk.Spec{
								Name: "deepest",
								Type: sdk.TypeInt,
							},
						},
					},
				},
			},
		},
	}

	out := roundTrip(t, in)
	if !reflect.DeepEqual(in, out) {
		t.Errorf("structure did not round-trip:\n in: %+v\nout: %+v", in, out)
	}
}

func TestSpecRoundTripNils(t *testing.T) {
	p, err := specToProto(nil)
	if err != nil || p != nil {
		t.Errorf("specToProto(nil) = %v, %v", p, err)
	}
	s, err := specFromProto(nil)
	if err != nil || s != nil {
		t.Errorf("specFromProto(nil) = %v, %v", s, err)
	}

	ps, err := specsToProto(nil)
	if err != nil || ps != nil {
		t.Errorf("specsToProto(nil) = %v, %v", ps, err)
	}
	ss, err := specsFromProto(nil)
	if err != nil || ss != nil {
		t.Errorf("specsFromProto(nil) = %v, %v", ss, err)
	}
}

func TestSpecDefaultRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want any
	}{
		{"string", "!", "!"},
		{"empty string", "", ""},
		{"int", 1, int64(1)},
		{"negative int", -42, int64(-42)},
		{"int64 max", int64(math.MaxInt64), int64(math.MaxInt64)},
		{"int64 min", int64(math.MinInt64), int64(math.MinInt64)},
		// 2^53+1: not representable in float64; must survive via the
		// UseNumber+Int64 path.
		{"beyond float53", int64(9007199254740993), int64(9007199254740993)},
		{"float", 1.5, 1.5},
		{"bool", true, true},
		{"nil stays absent", nil, nil},
		{"list", []string{"a", "b"}, []any{"a", "b"}},
		{"list of ints", []int{1, 2}, []any{int64(1), int64(2)}},
		{"map", map[string]any{"k": 1, "s": "v"}, map[string]any{"k": int64(1), "s": "v"}},
		{"nested", map[string]any{"l": []any{1.5, "x"}}, map[string]any{"l": []any{1.5, "x"}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := roundTrip(t, &sdk.Spec{Name: "d", Type: sdk.TypeString, Default: c.in})
			if !reflect.DeepEqual(out.Default, c.want) {
				t.Errorf("default %#v round-tripped to %#v, want %#v", c.in, out.Default, c.want)
			}
		})
	}
}

// Edge cases where the round trip is lossy or surprising. These document
// current behavior; failures here are findings, not necessarily bugs.
func TestSpecDefaultEdges(t *testing.T) {
	t.Run("integral float honors TypeFloat", func(t *testing.T) {
		// JSON renders 2.0 as `2`; the spec's declared type disambiguates.
		out := roundTrip(t, &sdk.Spec{Name: "f", Type: sdk.TypeFloat, Default: 2.0})
		if got, ok := out.Default.(float64); !ok || got != 2.0 {
			t.Errorf("float64(2.0) round-tripped to %#v (%T), not float64", out.Default, out.Default)
		}
	})

	t.Run("integral float in typed list", func(t *testing.T) {
		out := roundTrip(t, &sdk.Spec{
			Name: "fs", Type: sdk.TypeList,
			ElemType: &sdk.Spec{Name: "f", Type: sdk.TypeFloat},
			Default:  []float64{1.0, 2.5},
		})
		if want := []any{1.0, 2.5}; !reflect.DeepEqual(out.Default, want) {
			t.Errorf("float list round-tripped to %#v, want %#v", out.Default, want)
		}
	})

	t.Run("integral float in object field", func(t *testing.T) {
		out := roundTrip(t, &sdk.Spec{
			Name: "o", Type: sdk.TypeObject,
			Fields: []*sdk.Spec{
				{Name: "ratio", Type: sdk.TypeFloat},
				{Name: "count", Type: sdk.TypeInt},
			},
			Default: map[string]any{"ratio": 3.0, "count": 3, "untyped": 3},
		})
		want := map[string]any{"ratio": 3.0, "count": int64(3), "untyped": int64(3)}
		if !reflect.DeepEqual(out.Default, want) {
			t.Errorf("object default round-tripped to %#v, want %#v", out.Default, want)
		}
	})

	t.Run("integral float in map elem", func(t *testing.T) {
		out := roundTrip(t, &sdk.Spec{
			Name: "m", Type: sdk.TypeMap,
			ElemType: &sdk.Spec{Name: "v", Type: sdk.TypeFloat},
			Default:  map[string]any{"a": 4.0},
		})
		if want := map[string]any{"a": 4.0}; !reflect.DeepEqual(out.Default, want) {
			t.Errorf("map default round-tripped to %#v, want %#v", out.Default, want)
		}
	})

	t.Run("TypeInt overflow falls back", func(t *testing.T) {
		// Declared int, but the value exceeds int64: heuristic degradation
		// (float64) still applies rather than erroring.
		out := roundTrip(t, &sdk.Spec{Name: "i", Type: sdk.TypeInt, Default: uint64(math.MaxUint64)})
		if _, ok := out.Default.(float64); !ok {
			t.Errorf("overflowing TypeInt default = %#v (%T), want float64 fallback", out.Default, out.Default)
		}
	})

	t.Run("uint64 beyond int64 degrades to float64", func(t *testing.T) {
		out := roundTrip(t, &sdk.Spec{Name: "u", Type: sdk.TypeInt, Default: uint64(math.MaxUint64)})
		// Int64 overflows; Float64 succeeds with precision loss.
		if got, ok := out.Default.(float64); !ok || got != 1.8446744073709552e19 {
			t.Errorf("MaxUint64 round-tripped to %#v (%T)", out.Default, out.Default)
		}
	})

	t.Run("typed nil default becomes absent", func(t *testing.T) {
		// Default: (*int)(nil) is a non-nil interface, marshals to JSON
		// null, decodes to v=nil — so Default comes back untyped nil.
		out := roundTrip(t, &sdk.Spec{Name: "n", Type: sdk.TypeInt, Default: (*int)(nil)})
		if out.Default != nil {
			t.Errorf("typed-nil default round-tripped to %#v", out.Default)
		}
	})

	t.Run("json.Number passthrough", func(t *testing.T) {
		out := roundTrip(t, &sdk.Spec{Name: "j", Type: sdk.TypeInt, Default: json.Number("7")})
		if out.Default != int64(7) {
			t.Errorf("json.Number(7) round-tripped to %#v (%T)", out.Default, out.Default)
		}
	})
}

func TestSpecToProtoErrors(t *testing.T) {
	cases := []struct {
		name string
		def  any
	}{
		{"func", func() {}},
		{"chan", make(chan int)},
		{"NaN", math.NaN()},
		{"+Inf", math.Inf(1)},
		{"cyclic", func() any {
			m := map[string]any{}
			m["self"] = m
			return m
		}()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := specToProto(&sdk.Spec{Name: "bad", Default: c.def})
			if err == nil {
				t.Errorf("specToProto with %s default: expected error", c.name)
			}
		})
	}

	// An unmarshalable default nested deep in the tree must surface too.
	_, err := specToProto(&sdk.Spec{
		Name: "outer",
		Type: sdk.TypeList,
		ElemType: &sdk.Spec{
			Name:    "inner",
			Default: func() {},
		},
	})
	if err == nil {
		t.Error("unmarshalable default in ElemType: expected error")
	}
	_, err = specsToProto([]*sdk.Spec{{Name: "f", Fields: []*sdk.Spec{{Name: "g", Default: math.NaN()}}}})
	if err == nil {
		t.Error("unmarshalable default in Fields: expected error")
	}
}

func TestSpecFromProtoErrors(t *testing.T) {
	p, err := specToProto(&sdk.Spec{Name: "ok", Type: sdk.TypeString})
	if err != nil {
		t.Fatalf("specToProto: %v", err)
	}
	p.DefaultJson = []byte(`{"unterminated`)
	if _, err := specFromProto(p); err == nil {
		t.Error("malformed default_json: expected error")
	}

	// Trailing garbage after a valid value.
	p.DefaultJson = []byte(`1 garbage`)
	if _, err := specFromProto(p); err == nil {
		t.Error("trailing garbage in default_json: expected error")
	}
}

func TestNativeDefault(t *testing.T) {
	cases := []struct {
		name string
		in   any
		spec *sdk.Spec
		want any
	}{
		// untyped heuristic (spec nil)
		{"integral", json.Number("42"), nil, int64(42)},
		{"negative", json.Number("-7"), nil, int64(-7)},
		{"float", json.Number("1.25"), nil, 1.25},
		{"exponent", json.Number("1e3"), nil, float64(1000)},
		{"beyond int64", json.Number("18446744073709551615"), nil, 1.8446744073709552e19},
		{"beyond float64", json.Number("1e999"), nil, "1e999"},
		{"non-number passthrough", "str", nil, "str"},
		{"nil", nil, nil, nil},
		{"bool", true, nil, true},
		{"list", []any{json.Number("1"), "x"}, nil, []any{int64(1), "x"}},
		{"map", map[string]any{"n": json.Number("2.5")}, nil, map[string]any{"n": 2.5}},
		{"nested", []any{map[string]any{"n": json.Number("3")}}, nil, []any{map[string]any{"n": int64(3)}}},
		// typed
		{"TypeFloat integral", json.Number("2"), &sdk.Spec{Type: sdk.TypeFloat}, 2.0},
		{"TypeInt", json.Number("2"), &sdk.Spec{Type: sdk.TypeInt}, int64(2)},
		{"TypeFloat beyond float64 falls back", json.Number("1e999"), &sdk.Spec{Type: sdk.TypeFloat}, "1e999"},
		{"TypeString ignores number typing", json.Number("2"), &sdk.Spec{Type: sdk.TypeString}, int64(2)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nativeDefault(c.in, c.spec); !reflect.DeepEqual(got, c.want) {
				t.Errorf("nativeDefault(%#v, %+v) = %#v, want %#v", c.in, c.spec, got, c.want)
			}
		})
	}
}

func TestRangeRoundTrip(t *testing.T) {
	in := sdk.SourceRange{SourceName: "p.psy", StartLine: 1, StartCol: 2, EndLine: 3, EndCol: 4}
	if out := rangeFromProto(rangeToProto(in)); out != in {
		t.Errorf("range round-trip = %+v, want %+v", out, in)
	}
	if out := rangeFromProto(nil); out != (sdk.SourceRange{}) {
		t.Errorf("rangeFromProto(nil) = %+v, want zero", out)
	}
	if out := rangeFromProto(rangeToProto(sdk.SourceRange{})); out != (sdk.SourceRange{}) {
		t.Errorf("zero range round-trip = %+v", out)
	}
}

func TestDescriptorsRoundTrip(t *testing.T) {
	in := []sdk.ResourceDescriptor{
		{Name: "a", Kinds: sdk.PRODUCER | sdk.TRANSFORMER, Spec: []*sdk.Spec{{Name: "x", Type: sdk.TypeBool, Default: false}}},
		{Name: "b", Kinds: 0, Spec: nil},
	}
	p, err := descriptorsToProto(in)
	if err != nil {
		t.Fatalf("descriptorsToProto: %v", err)
	}
	out, err := descriptorsFromProto(p)
	if err != nil {
		t.Fatalf("descriptorsFromProto: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("descriptors did not round-trip:\n in: %+v\nout: %+v", in, out)
	}
}

func TestDescriptorsErrors(t *testing.T) {
	// A bad default inside a descriptor's spec must surface, wrapped with
	// the resource name.
	_, err := descriptorsToProto([]sdk.ResourceDescriptor{
		{Name: "broken", Spec: []*sdk.Spec{{Name: "d", Default: math.NaN()}}},
	})
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("descriptorsToProto bad default: err = %v, want mention of resource name", err)
	}

	good, err := descriptorsToProto([]sdk.ResourceDescriptor{
		{Name: "broken", Spec: []*sdk.Spec{{Name: "d", Type: sdk.TypeInt}}},
	})
	if err != nil {
		t.Fatalf("descriptorsToProto: %v", err)
	}
	good[0].Spec[0].DefaultJson = []byte(`{bad`)
	if _, err := descriptorsFromProto(good); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("descriptorsFromProto bad default_json: err = %v, want mention of resource name", err)
	}
}
