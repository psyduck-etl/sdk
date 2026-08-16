package data

import (
	"errors"
	"testing"
)

// onerror.go had no test coverage at all before this file: OnError,
// ParseOnError, Handle, String, and WrapHandlerErr were never called from
// any test.

func TestParseOnErrorDefaultsToRaise(t *testing.T) {
	h, err := ParseOnError("")
	if err != nil {
		t.Fatalf(`ParseOnError(""): %v`, err)
	}
	want := errors.New("boom")
	if got := h(want); got != want {
		t.Errorf("default handler = %v, want %v (raise forwards unchanged)", got, want)
	}
}

func TestParseOnErrorRaise(t *testing.T) {
	h, err := ParseOnError("raise")
	if err != nil {
		t.Fatalf("ParseOnError(raise): %v", err)
	}
	want := errors.New("boom")
	if got := h(want); got != want {
		t.Errorf("raise handler = %v, want %v", got, want)
	}
}

func TestParseOnErrorDrop(t *testing.T) {
	h, err := ParseOnError("drop")
	if err != nil {
		t.Fatalf("ParseOnError(drop): %v", err)
	}
	if got := h(errors.New("boom")); got != nil {
		t.Errorf("drop handler = %v, want nil", got)
	}
}

func TestParseOnErrorUnknown(t *testing.T) {
	if _, err := ParseOnError("explode"); err == nil {
		t.Error("expected error for an unknown on-error mode")
	}
}

func TestRaiseAndDropVars(t *testing.T) {
	want := errors.New("boom")
	if got := Raise(want); got != want {
		t.Errorf("Raise = %v, want %v", got, want)
	}
	if got := Drop(want); got != nil {
		t.Errorf("Drop = %v, want nil", got)
	}
}

func TestOnErrorKindString(t *testing.T) {
	if ON_ERROR_RAISE.String() != "raise" {
		t.Errorf("ON_ERROR_RAISE.String() = %q, want raise", ON_ERROR_RAISE.String())
	}
	if ON_ERROR_DROP.String() != "drop" {
		t.Errorf("ON_ERROR_DROP.String() = %q, want drop", ON_ERROR_DROP.String())
	}
}

func TestOnErrorKindInvalidPanics(t *testing.T) {
	// ParseOnError is the only path that produces an onErrorKind, so an
	// out-of-band value can only arise from deliberate abuse (a raw
	// conversion) — Handle and String panic rather than silently doing the
	// wrong thing.
	invalid := onErrorKind(99)

	assertPanics(t, "Handle", func() { invalid.Handle(errors.New("x")) })
	assertPanics(t, "String", func() { _ = invalid.String() })
}

func TestWrapHandlerErr(t *testing.T) {
	err := WrapHandlerErr(errors.New("original"), errors.New("handling"))
	want := `while handling error "original": encountered error "handling"`
	if err.Error() != want {
		t.Errorf("WrapHandlerErr = %q, want %q", err.Error(), want)
	}
}

func assertPanics(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: expected panic on invalid onErrorKind", name)
		}
	}()
	fn()
}
