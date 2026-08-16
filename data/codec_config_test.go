package data

import (
	"testing"

	"github.com/psyduck-etl/sdk"
)

func init() {
	// codec_config.go's Decode/Encode route through the bound sdk.Codec, so
	// exercising them needs a registered factory — the same one the real
	// host installs via sdk.RegisterCodecs(data.Codec) at startup.
	sdk.RegisterCodecs(Codec)
}

func TestInputCodecDecodeValue(t *testing.T) {
	c := &InputCodec{Accept: "json"}

	v, err := c.DecodeValue([]byte(`{"a":1,"b":[2,3]}`))
	if err != nil {
		t.Fatalf("DecodeValue: %v", err)
	}
	obj, ok := v.(Object)
	if !ok {
		t.Fatalf("want Object, got %T (%s)", v, v.Kind())
	}
	if obj["a"].Kind() != KindLit {
		t.Errorf("want a to decode as Lit, got %s", obj["a"].Kind())
	}
	if obj["b"].Kind() != KindList {
		t.Errorf("want b to decode as List, got %s", obj["b"].Kind())
	}
}

func TestInputCodecDecodeValueDoesNotRequireBind(t *testing.T) {
	c := &InputCodec{Accept: "utf-8"}
	// Deliberately not calling Bind() — DecodeValue resolves the spec
	// through the data package's own chain, not the bound sdk.Codec.
	v, err := c.DecodeValue([]byte("hello"))
	if err != nil {
		t.Fatalf("DecodeValue: %v", err)
	}
	if v.String() != "hello" {
		t.Errorf("want %q, got %q", "hello", v.String())
	}
}

func TestOutputCodecEncodeValue(t *testing.T) {
	c := &OutputCodec{Emit: "json"}

	out, err := c.EncodeValue(Object{"a": Str("b")})
	if err != nil {
		t.Fatalf("EncodeValue: %v", err)
	}
	if string(out) != `{"a":"b"}` {
		t.Errorf("want %s, got %s", `{"a":"b"}`, out)
	}
}

func TestCodecValueRoundTripSkipsNative(t *testing.T) {
	// DecodeValue -> EncodeValue should preserve a Value's shape exactly,
	// the same way Decode/Encode round-trips through native `any` does, but
	// without ever lowering through Native()/fromNative() in between.
	in := &InputCodec{Accept: "json"}
	out := &OutputCodec{Emit: "json"}

	src := []byte(`{"list":[1,"two",true,null]}`)
	v, err := in.DecodeValue(src)
	if err != nil {
		t.Fatalf("DecodeValue: %v", err)
	}
	got, err := out.EncodeValue(v)
	if err != nil {
		t.Fatalf("EncodeValue: %v", err)
	}
	if string(got) != string(src) {
		t.Errorf("want %s, got %s", src, got)
	}
}

func TestInputCodecSparse(t *testing.T) {
	if (&InputCodec{Accept: "string"}).Sparse() != true {
		t.Errorf("want Sparse() true for Accept=%q", "string")
	}
	if (&InputCodec{Accept: "json"}).Sparse() != false {
		t.Errorf("want Sparse() false for Accept=%q", "json")
	}
}

func TestOutputCodecSparse(t *testing.T) {
	if (&OutputCodec{Emit: "string"}).Sparse() != true {
		t.Errorf("want Sparse() true for Emit=%q", "string")
	}
	if (&OutputCodec{Emit: "json"}).Sparse() != false {
		t.Errorf("want Sparse() false for Emit=%q", "json")
	}
}

func TestIsTerminalRef(t *testing.T) {
	if IsTerminalRef("string") != true {
		t.Errorf("want IsTerminalRef(%q) true", "string")
	}
	if IsTerminalRef("json") != false {
		t.Errorf("want IsTerminalRef(%q) false", "json")
	}
}

func TestInputCodecDecodeVsDecodeValue(t *testing.T) {
	c := &InputCodec{Accept: "json"}
	if err := c.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	native, err := c.Decode([]byte(`{"a":[1,2]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if _, ok := native.(map[string]any); !ok {
		t.Fatalf("Decode: want map[string]any, got %T", native)
	}

	v, err := c.DecodeValue([]byte(`{"a":[1,2]}`))
	if err != nil {
		t.Fatalf("DecodeValue: %v", err)
	}
	if _, ok := v.(Object); !ok {
		t.Fatalf("DecodeValue: want Object, got %T", v)
	}
}
