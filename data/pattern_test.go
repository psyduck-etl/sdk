package data

import (
	"bytes"
	"fmt"
	"testing"
)

// Decode/Encode in codec.go call codec.decode/codec.encode directly rather
// than going through Chain, so exercise Chain's multi-pattern branch here.
func TestChainMultiPatternError(t *testing.T) {
	upper := func(b []byte) ([]byte, error) { return bytes.ToUpper(b), nil }
	failing := func(b []byte) ([]byte, error) { return nil, fmt.Errorf("boom") }

	ok := Chain(upper, upper)
	out, err := ok([]byte("hi"))
	if err != nil || string(out) != "HI" {
		t.Errorf("Chain(upper,upper) = %q err=%v, want HI nil", out, err)
	}

	failFirst := Chain(failing, upper)
	if _, err := failFirst([]byte("hi")); err == nil {
		t.Error("Chain(failing,upper) expected error from the first step")
	}

	failSecond := Chain(upper, failing)
	if _, err := failSecond([]byte("hi")); err == nil {
		t.Error("Chain(upper,failing) expected error from the second step")
	}
}

// Registry.Decode/Encode are the public "give me a closure" surface for
// plugin authors that build a Pattern directly from a spec string; nothing
// in the SDK's own code calls them (Decode/Encode in codec.go walk
// Patterns.codecs by hand instead).
func TestRegistryDecodeEncodeChain(t *testing.T) {
	dec, err := Patterns.Decode("base64|hex")
	if err != nil {
		t.Fatalf("Patterns.Decode: %v", err)
	}
	enc, err := Patterns.Encode("base64|hex")
	if err != nil {
		t.Fatalf("Patterns.Encode: %v", err)
	}

	// Build the payload a "base64|hex" decode chain expects: raw -> hex ->
	// base64 (decode undoes base64 first, then hex, so encode must apply
	// them in that order to match).
	orig := []byte("hello world")
	hexEncoded, err := Patterns.codecs["hex"].encode(orig)
	if err != nil {
		t.Fatalf("hex encode seed: %v", err)
	}
	b64OfHex, err := Patterns.codecs["base64"].encode(hexEncoded)
	if err != nil {
		t.Fatalf("base64 encode seed: %v", err)
	}

	decoded, err := dec(b64OfHex)
	if err != nil {
		t.Fatalf("chained decode: %v", err)
	}
	if string(decoded) != string(orig) {
		t.Errorf("chained decode = %q, want %q", decoded, orig)
	}

	// Encode is the inverse of decode: it must reverse the spec order (hex
	// first, then base64) to reproduce the same bytes a "hex then base64"
	// encode would.
	reencoded, err := enc(orig)
	if err != nil {
		t.Fatalf("chained encode: %v", err)
	}
	if string(reencoded) != string(b64OfHex) {
		t.Errorf("chained encode = %q, want %q (base64|hex must encode hex-then-base64)", reencoded, b64OfHex)
	}
}

func TestRegistryUnknownCodec(t *testing.T) {
	if _, err := Patterns.Decode("nope"); err == nil {
		t.Error("expected Patterns.Decode error for an unknown codec")
	}
	if _, err := Patterns.Encode("nope"); err == nil {
		t.Error("expected Patterns.Encode error for an unknown codec")
	}
}
