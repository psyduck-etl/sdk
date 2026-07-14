package data

import "fmt"

// onErrorKind is a closed set of on-error handling behaviors. ParseOnError is
// the only path that produces one from a config string, so any onErrorKind
// value elsewhere in the program is guaranteed to be one of the two
// constants below — Handle's default case is unreachable except through
// deliberate abuse (an out-of-band int conversion), so it panics rather than
// returning an error.
type onErrorKind int

const (
	// ON_ERROR_RAISE forwards the error unchanged (config spelling "raise").
	ON_ERROR_RAISE onErrorKind = iota
	// ON_ERROR_DROP swallows the error and drops the message (config
	// spelling "drop").
	ON_ERROR_DROP
)

// OnError decides what happens when an operation fails on a message. It
// receives the error and returns what should propagate: a non-nil error is
// forwarded (core propagates whatever a transformer returns unconditionally,
// so "raise" and "drop" need no separate downstream path); nil swallows it
// and drops the message.
//
// A handler that itself fails while handling (logging, a metrics call, ...)
// returns that failure rather than silently discarding one error or the
// other — Drop surfaces the handling error in its place; a handler built on
// top of Raise should compose the two with WrapHandlerErr.
//
// OnError is a plain function type, not a single-method interface — the same
// choice sdk.Parser and http.HandlerFunc make for a one-method contract. Every
// call site binds it by capturing the value in a closure at construction
// time; there are no long-lived transformer structs to hang an interface
// field off of, and a closure already gives a handler private state (a drop
// counter, a log destination) without an interface's indirection.
type OnError func(error) error

// Handle turns a validated onErrorKind into its OnError behavior.
func (kind onErrorKind) Handle(err error) error {
	switch kind {
	case ON_ERROR_RAISE:
		return err
	case ON_ERROR_DROP:
		return nil
	default:
		panic(fmt.Sprintf("data: onErrorKind(%d) is not a valid on-error kind", int(kind)))
	}
}

// String is the config spelling of kind — the single source of truth
// ParseOnError matches incoming strings against, so the spelling is never
// duplicated as a bare literal.
func (kind onErrorKind) String() string {
	switch kind {
	case ON_ERROR_RAISE:
		return "raise"
	case ON_ERROR_DROP:
		return "drop"
	default:
		panic(fmt.Sprintf("data: onErrorKind(%d) is not a valid on-error kind", int(kind)))
	}
}

// Raise and Drop are the OnError callbacks for the two built-in kinds,
// exposed directly so a caller that already knows which kind it wants (e.g.
// the default when on-error is unset) doesn't have to round-trip through
// ParseOnError.
var (
	Raise OnError = ON_ERROR_RAISE.Handle
	Drop  OnError = ON_ERROR_DROP.Handle
)

// WrapHandlerErr composes an error encountered while handling with the
// original, so a handler that performs a fallible action doesn't have to
// choose which one to discard.
func WrapHandlerErr(original, handling error) error {
	return fmt.Errorf("while handling error %q: encountered error %q", original, handling)
}

// ParseOnError parses the config spelling of an error handler (ON_ERROR_RAISE
// or ON_ERROR_DROP's String(), "" defaults to ON_ERROR_RAISE) into an OnError
// callback. s is matched only against the actual const values' spellings (or
// the "" default) — never a bare literal that could drift from them — so this
// is the only place a raw string is allowed to determine on-error behavior;
// everywhere else works in terms of onErrorKind or the OnError callback
// Handle produces from one.
func ParseOnError(s string) (OnError, error) {
	if s == "" {
		return ON_ERROR_RAISE.Handle, nil
	}
	for _, kind := range []onErrorKind{ON_ERROR_RAISE, ON_ERROR_DROP} {
		if kind.String() == s {
			return kind.Handle, nil
		}
	}
	return nil, fmt.Errorf("unknown error mode %q (want %q or %q)", s, ON_ERROR_RAISE, ON_ERROR_DROP)
}
