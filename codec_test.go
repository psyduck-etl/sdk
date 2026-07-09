package sdk

import (
	"errors"
	"strings"
	"testing"
)

// stubCodec is a tiny codec that lets us observe which spec the factory
// resolved. It doesn't need to actually encode anything meaningful.
type stubCodec struct{ spec string }

func (c stubCodec) Decode(b []byte) (any, error) { return string(b) + "@" + c.spec, nil }
func (c stubCodec) Encode(v any) ([]byte, error) { return []byte(v.(string) + "@" + c.spec), nil }

func TestCodec_DefaultFactoryErrors(t *testing.T) {
	// Save and restore so this test doesn't leak state to siblings.
	saved := factory
	t.Cleanup(func() { factory = saved })
	factory = func(spec string) (Codec, error) {
		return nil, errors.New("no codec factory registered; cannot resolve " + spec)
	}

	_, err := GetCodec("json")
	if err == nil || !strings.Contains(err.Error(), "no codec factory") {
		t.Fatalf("want unregistered-factory error, got %v", err)
	}
}

func TestCodec_RegisterAndResolve(t *testing.T) {
	saved := factory
	t.Cleanup(func() { factory = saved })

	RegisterCodecs(func(spec string) (Codec, error) {
		return stubCodec{spec}, nil
	})

	c, err := GetCodec("json")
	if err != nil {
		t.Fatalf("GetCodec: %v", err)
	}
	got, err := c.Encode("hi")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if string(got) != "hi@json" {
		t.Fatalf("Encode = %q, want %q", got, "hi@json")
	}
}

func TestCodec_RegisterNilIsNoop(t *testing.T) {
	saved := factory
	t.Cleanup(func() { factory = saved })

	RegisterCodecs(func(spec string) (Codec, error) { return stubCodec{spec}, nil })
	RegisterCodecs(nil) // nil should not clobber a real factory

	c, err := GetCodec("anything")
	if err != nil {
		t.Fatalf("GetCodec: %v", err)
	}
	if c.(stubCodec).spec != "anything" {
		t.Fatalf("factory replaced by nil; spec = %q", c.(stubCodec).spec)
	}
}
