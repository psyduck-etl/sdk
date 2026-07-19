package sdk

import (
	"errors"
	"testing"
)

func TestAcceptConfigBind(t *testing.T) {
	// Register a test codec factory that knows only "json" and "string".
	oldFactory := factory
	defer func() { factory = oldFactory }()
	RegisterCodecs(func(spec string) (Codec, error) {
		switch spec {
		case "json":
			return &testCodec{}, nil
		case "string":
			return &testCodec{}, nil
		default:
			return nil, errors.New("unknown codec")
		}
	})

	tests := []struct {
		name    string
		accept  string
		wantErr bool
	}{
		{"valid json", "json", false},
		{"valid string", "string", false},
		{"empty", "", true},
		{"unknown", "yaml", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AcceptConfig{Accept: tt.accept}
			err := c.Bind()
			if (err != nil) != tt.wantErr {
				t.Errorf("Bind() error = %v, want error = %v", err, tt.wantErr)
			}
		})
	}
}

func TestAcceptConfigDecode(t *testing.T) {
	oldFactory := factory
	defer func() { factory = oldFactory }()
	RegisterCodecs(func(spec string) (Codec, error) {
		if spec == "json" {
			return &testCodec{}, nil
		}
		return nil, errors.New("unknown codec")
	})

	t.Run("decode before bind", func(t *testing.T) {
		c := &AcceptConfig{Accept: "json"}
		_, err := c.Decode([]byte("{}"))
		if err == nil {
			t.Error("Decode() before Bind() should fail")
		}
	})

	t.Run("decode after bind", func(t *testing.T) {
		c := &AcceptConfig{Accept: "json"}
		if err := c.Bind(); err != nil {
			t.Fatalf("Bind() failed: %v", err)
		}
		v, err := c.Decode([]byte("{}"))
		if err != nil {
			t.Errorf("Decode() failed: %v", err)
		}
		if v != testValue {
			t.Errorf("Decode() returned %v, want %v", v, testValue)
		}
	})
}

func TestEmitConfigBind(t *testing.T) {
	oldFactory := factory
	defer func() { factory = oldFactory }()
	RegisterCodecs(func(spec string) (Codec, error) {
		switch spec {
		case "json":
			return &testCodec{}, nil
		case "csv":
			return &testCodec{}, nil
		default:
			return nil, errors.New("unknown codec")
		}
	})

	tests := []struct {
		name    string
		emit    string
		wantErr bool
	}{
		{"valid json", "json", false},
		{"valid csv", "csv", false},
		{"empty", "", true},
		{"unknown", "protobuf", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &EmitConfig{Emit: tt.emit}
			err := c.Bind()
			if (err != nil) != tt.wantErr {
				t.Errorf("Bind() error = %v, want error = %v", err, tt.wantErr)
			}
		})
	}
}

func TestEmitConfigEncode(t *testing.T) {
	oldFactory := factory
	defer func() { factory = oldFactory }()
	RegisterCodecs(func(spec string) (Codec, error) {
		if spec == "json" {
			return &testCodec{}, nil
		}
		return nil, errors.New("unknown codec")
	})

	t.Run("encode before bind", func(t *testing.T) {
		c := &EmitConfig{Emit: "json"}
		_, err := c.Encode(map[string]any{})
		if err == nil {
			t.Error("Encode() before Bind() should fail")
		}
	})

	t.Run("encode after bind", func(t *testing.T) {
		c := &EmitConfig{Emit: "json"}
		if err := c.Bind(); err != nil {
			t.Fatalf("Bind() failed: %v", err)
		}
		b, err := c.Encode(map[string]any{})
		if err != nil {
			t.Errorf("Encode() failed: %v", err)
		}
		if string(b) != testBytes {
			t.Errorf("Encode() returned %q, want %q", b, testBytes)
		}
	})
}

func TestCodecConfigBind(t *testing.T) {
	oldFactory := factory
	defer func() { factory = oldFactory }()
	RegisterCodecs(func(spec string) (Codec, error) {
		switch spec {
		case "json", "csv", "string":
			return &testCodec{}, nil
		default:
			return nil, errors.New("unknown codec")
		}
	})

	tests := []struct {
		name    string
		accept  string
		emit    string
		wantErr bool
	}{
		{"both valid", "json", "csv", false},
		{"accept empty", "", "json", true},
		{"emit empty", "json", "", true},
		{"accept unknown", "yaml", "json", true},
		{"emit unknown", "json", "protobuf", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CodecConfig{
				AcceptConfig: AcceptConfig{Accept: tt.accept},
				EmitConfig:   EmitConfig{Emit: tt.emit},
			}
			err := c.Bind()
			if (err != nil) != tt.wantErr {
				t.Errorf("Bind() error = %v, want error = %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsTerminalRef(t *testing.T) {
	tests := []struct {
		spec string
		want bool
	}{
		{"string", true},
		{"json", false},
		{"csv", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			if got := IsTerminalRef(tt.spec); got != tt.want {
				t.Errorf("IsTerminalRef(%q) = %v, want %v", tt.spec, got, tt.want)
			}
		})
	}
}

// testCodec is a stub for testing that returns fixed values.
type testCodec struct{}

const testValue = "test"
const testBytes = "test-encoded"

func (tc *testCodec) Decode(b []byte) (any, error) {
	return testValue, nil
}

func (tc *testCodec) Encode(v any) ([]byte, error) {
	return []byte(testBytes), nil
}
