package data

import "testing"

// ── normSlice exact-bound cases ─────────────────────────────────────────
//
// TestBytesSlice (data_test.go) covers the interior and overflow cases;
// these cover the exact-bound edges (lo/hi landing precisely on 0 or n),
// where the clamped value doesn't change but trunc must still be false.

func TestSliceStartAtZero(t *testing.T) {
	b := Bytes("abcde")
	out, trunc := b.Slice(0, 3, 1)
	if string(out.Bytes()) != "abc" || trunc {
		t.Errorf("Slice(0,3,1) = %q trunc=%v, want %q false", out.Bytes(), trunc, "abc")
	}
}

func TestSliceStartAtLen(t *testing.T) {
	b := Bytes("abcde")
	out, trunc := b.Slice(5, 5, 1) // start == len(b): a valid empty slice, not a clamp
	if string(out.Bytes()) != "" || trunc {
		t.Errorf("Slice(5,5,1) = %q trunc=%v, want empty false", out.Bytes(), trunc)
	}
}

func TestSliceStopAtZero(t *testing.T) {
	b := Bytes("abcde")
	out, trunc := b.Slice(0, 0, 1)
	if string(out.Bytes()) != "" || trunc {
		t.Errorf("Slice(0,0,1) = %q trunc=%v, want empty false", out.Bytes(), trunc)
	}
}

func TestSliceStopAtLen(t *testing.T) {
	b := Bytes("abcde")
	out, trunc := b.Slice(0, 5, 1) // stop == len(b): the full slice, not a clamp
	if string(out.Bytes()) != "abcde" || trunc {
		t.Errorf("Slice(0,5,1) = %q trunc=%v, want %q false", out.Bytes(), trunc, "abcde")
	}
}

// ── Chunk edge cases ─────────────────────────────────────────────────────

func TestChunkSizeZero(t *testing.T) {
	// size <= 0 returns the whole slice as a single chunk. Weakening the
	// guard to size < 0 would let size == 0 fall into the windowing loop,
	// which never advances (i += 0).
	b := Bytes("abcdef")
	out := b.Chunk(0, false)
	if len(out) != 1 || out[0].String() != "abcdef" {
		t.Errorf("Chunk(0,...) = %v, want a single full chunk", chunkStrings(out))
	}
}

func TestChunkEvenlyDivisibleKeepTail(t *testing.T) {
	// Input length is an exact multiple of size, so there is no short tail.
	// Weakening the loop bound (i < len(s) -> i <= len(s)) would run one
	// extra iteration and append a trailing empty chunk when keepTail=true.
	b := Bytes("abcdef") // len 6, size 3: divides evenly
	out := b.Chunk(3, true)
	want := []string{"abc", "def"}
	if got := chunkStrings(out); !eqStrings(got, want) {
		t.Errorf("Chunk(3,true) on evenly-divisible input = %v, want %v", got, want)
	}
}

func TestChunkEvenlyDivisibleDropTail(t *testing.T) {
	// Same evenly-divisible input with keepTail=false: weakening the
	// "end > len(s)" check to ">=" would wrongly treat the last full chunk
	// as a short tail and drop it.
	b := Bytes("abcdef")
	out := b.Chunk(3, false)
	want := []string{"abc", "def"}
	if got := chunkStrings(out); !eqStrings(got, want) {
		t.Errorf("Chunk(3,false) on evenly-divisible input = %v, want %v", got, want)
	}
}

// ── Every zero-value defaults ────────────────────────────────────────────

func TestEveryStepZeroDefaultsToOne(t *testing.T) {
	// step <= 0 resets to 1. Weakening that to step < 0 would leave step at
	// 0 for this input, which never advances the loop (i += 0).
	b := Bytes("abcde")
	zero := b.Every(0, 2)
	one := b.Every(1, 2)
	if got, want := chunkStrings(zero), chunkStrings(one); !eqStrings(got, want) {
		t.Errorf("Every(step=0,...) = %v, want same as step=1: %v", got, want)
	}
}

func TestEverySizeZeroDefaultsToOne(t *testing.T) {
	// size <= 0 resets to 1. Weakening that to size < 0 would leave size at
	// 0 for this input, producing a run of empty windows instead.
	b := Bytes("abcde")
	zero := b.Every(1, 0)
	one := b.Every(1, 1)
	if got, want := chunkStrings(zero), chunkStrings(one); !eqStrings(got, want) {
		t.Errorf("Every(size=0,...) = %v, want same as size=1: %v", got, want)
	}
}

// The following edge cases are deliberately not distinguished by tests
// because the alternate behavior is unobservable:
//
//   - normSlice's `hi < lo` guard (slice.go:30): at the only distinguishing
//     input, hi == lo, the guarded assignment `hi = lo` is a no-op either
//     way.
//   - Slice's `step <= 1` fast path (slice.go:41): for step == 1 the
//     windowed loop and the `s[lo:hi:hi]` fast path produce the same
//     elements in the same order.
//   - The `make(S, 0, ...)` capacity hints in Slice and Chunk (slice.go:44,
//     57) only pre-size an append target; append grows the backing array
//     as needed regardless.
