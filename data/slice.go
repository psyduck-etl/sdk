package data

// This file holds the concrete-type-preserving slice helpers — the "~[]E"
// idiom the Go standard library uses (slices.Clone etc.) so that slicing a
// Bytes yields a Bytes, a List yields a List, and so on. These are the typed
// fast-path: they work only where the concrete slice type is statically known
// (inside the continuous method bodies). string cannot join this set — a union
// of string and []T has no core type in Go — so Str converts to Runes at the
// boundary and back.

// normSlice normalizes start/stop against length n. Values are plain
// indices — a Continuous is not assumed to have a calculable end, so a
// negative value is not reinterpreted relative to one; it clamps to 0 like
// any other out-of-range bound. trunc reports whether the requested bounds
// fell outside [0,n] and had to be clamped.
func normSlice(n, start, stop int) (lo, hi int, trunc bool) {
	lo, hi = start, stop
	if lo < 0 {
		lo, trunc = 0, true
	}
	if lo > n {
		lo, trunc = n, true
	}
	if hi < 0 {
		hi, trunc = 0, true
	}
	if hi > n {
		hi, trunc = n, true
	}
	if hi < lo {
		hi = lo
	}
	return lo, hi, trunc
}

// Slice returns s[start:stop] taking every step-th element, preserving the
// concrete slice type S. It reports trunc=true when the requested bounds were
// clamped to the slice's length. A step <= 1 yields a contiguous sub-slice.
func Slice[S ~[]E, E any](s S, start, stop, step int) (S, bool) {
	lo, hi, trunc := normSlice(len(s), start, stop)
	if step <= 1 {
		return s[lo:hi:hi], trunc
	}
	out := make(S, 0, (hi-lo+step-1)/step)
	for i := lo; i < hi; i += step {
		out = append(out, s[i])
	}
	return out, trunc
}

// Chunk splits s into consecutive windows of size elements, preserving S. When
// the final window is short, keepTail decides whether it is kept or dropped.
func Chunk[S ~[]E, E any](s S, size int, keepTail bool) []S {
	if size <= 0 {
		return []S{s}
	}
	out := make([]S, 0, (len(s)+size-1)/size)
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			if !keepTail {
				break
			}
			end = len(s)
		}
		out = append(out, s[i:end:end])
	}
	return out
}

// Every yields sliding windows of size elements advanced by step, preserving
// S. Only full-length windows are emitted.
func Every[S ~[]E, E any](s S, step, size int) []S {
	if step <= 0 {
		step = 1
	}
	if size <= 0 {
		size = 1
	}
	var out []S
	for i := 0; i+size <= len(s); i += step {
		out = append(out, s[i:i+size:i+size])
	}
	return out
}
