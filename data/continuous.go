package data

// Continuous is the array-like half of the model: data that can only be
// meaningfully referenced by index or by linear pattern matching. Its
// self-returning ops are boxed as Continuous (not the concrete type) because
// that is the only form that spans string and the slice kinds uniformly;
// callers that need the concrete type back use As[T].
type Continuous interface {
	Value
	// Len reports the number of elements (bytes, runes, or list items).
	Len() int
	// By applies a byte-level Pattern (a codec/decoder chain) to the value's
	// bytes and returns the transformed continuous view.
	By(p Pattern) (Continuous, error)
	// Get returns a length-1 window at index n (staying continuous), ok=false
	// when n is out of range.
	Get(n int) (Continuous, bool)
	// Slice returns [start:stop:step]; trunc=true when the range overflowed
	// and was clamped.
	Slice(start, stop, step int) (Continuous, bool)
	// Chunk splits into size-length windows; keepTail keeps a short final one.
	Chunk(size int, keepTail bool) []Continuous
	// Every yields size-length sliding windows advanced by step.
	Every(step, size int) []Continuous
}

// box lifts a slice of a concrete continuous type into []Continuous.
func box[S Continuous](ss []S) []Continuous {
	out := make([]Continuous, len(ss))
	for i := range ss {
		out[i] = ss[i]
	}
	return out
}

// applyBy runs a Pattern over a value's bytes and returns the result as Bytes.
// Decoding a byte pattern (gzip, base64, …) is inherently a bytes→bytes op, so
// non-Bytes kinds drop into the bytes domain here.
func applyBy(v Value, p Pattern) (Continuous, error) {
	out, err := p(v.Bytes())
	if err != nil {
		return nil, err
	}
	return Bytes(out), nil
}

// ── Bytes ────────────────────────────────────────────────────────────────

// Bytes is continuous raw []byte.
type Bytes []byte

func (b Bytes) Kind() Kind     { return KindBytes }
func (b Bytes) String() string { return string(b) }
func (b Bytes) Bytes() []byte  { return b }
func (b Bytes) Len() int       { return len(b) }

func (b Bytes) By(p Pattern) (Continuous, error) { return applyBy(b, p) }

func (b Bytes) Get(n int) (Continuous, bool) {
	if n < 0 || n >= len(b) {
		return nil, false
	}
	return b[n : n+1], true
}

func (b Bytes) Slice(start, stop, step int) (Continuous, bool) {
	out, trunc := Slice(b, start, stop, step)
	return out, trunc
}

func (b Bytes) Chunk(size int, keepTail bool) []Continuous {
	return box(Chunk(b, size, keepTail))
}

func (b Bytes) Every(step, size int) []Continuous {
	return box(Every(b, step, size))
}

// ── Runes ────────────────────────────────────────────────────────────────

// Runes is continuous []rune — the rune-aware view used for text operations.
type Runes []rune

func (r Runes) Kind() Kind     { return KindRunes }
func (r Runes) String() string { return string(r) }
func (r Runes) Bytes() []byte  { return []byte(string(r)) }
func (r Runes) Len() int       { return len(r) }

func (r Runes) By(p Pattern) (Continuous, error) { return applyBy(r, p) }

func (r Runes) Get(n int) (Continuous, bool) {
	if n < 0 || n >= len(r) {
		return nil, false
	}
	return r[n : n+1], true
}

func (r Runes) Slice(start, stop, step int) (Continuous, bool) {
	out, trunc := Slice(r, start, stop, step)
	return out, trunc
}

func (r Runes) Chunk(size int, keepTail bool) []Continuous {
	return box(Chunk(r, size, keepTail))
}

func (r Runes) Every(step, size int) []Continuous {
	return box(Every(r, step, size))
}

// ── Str ──────────────────────────────────────────────────────────────────

// Str is continuous text. Index/slice/chunk operations are rune-aware: Str
// converts to Runes at the boundary (a union of string and []rune has no core
// type, so the ~[]E helpers cannot include string directly) and back.
type Str string

func (s Str) Kind() Kind     { return KindStr }
func (s Str) String() string { return string(s) }
func (s Str) Bytes() []byte  { return []byte(s) }
func (s Str) Len() int       { return len([]rune(s)) }

func (s Str) By(p Pattern) (Continuous, error) { return applyBy(s, p) }

func (s Str) Get(n int) (Continuous, bool) {
	rs := []rune(s)
	if n < 0 || n >= len(rs) {
		return nil, false
	}
	return Str(rs[n : n+1]), true
}

func (s Str) Slice(start, stop, step int) (Continuous, bool) {
	out, trunc := Slice(Runes(s), start, stop, step)
	return Str(out), trunc
}

func (s Str) Chunk(size int, keepTail bool) []Continuous {
	rs := Chunk(Runes(s), size, keepTail)
	out := make([]Continuous, len(rs))
	for i, r := range rs {
		out[i] = Str(r)
	}
	return out
}

func (s Str) Every(step, size int) []Continuous {
	rs := Every(Runes(s), step, size)
	out := make([]Continuous, len(rs))
	for i, r := range rs {
		out[i] = Str(r)
	}
	return out
}

// ── List ─────────────────────────────────────────────────────────────────

// List is a continuous, ordered sequence of Values (a JSON array).
type List []Value

func (l List) Kind() Kind    { return KindList }
func (l List) Len() int      { return len(l) }
func (l List) Bytes() []byte { return marshalJSON(l) }

func (l List) String() string { return string(l.Bytes()) }

func (l List) By(p Pattern) (Continuous, error) { return applyBy(l, p) }

func (l List) Get(n int) (Continuous, bool) {
	if n < 0 || n >= len(l) {
		return nil, false
	}
	return l[n : n+1], true
}

func (l List) Slice(start, stop, step int) (Continuous, bool) {
	out, trunc := Slice(l, start, stop, step)
	return out, trunc
}

func (l List) Chunk(size int, keepTail bool) []Continuous {
	return box(Chunk(l, size, keepTail))
}

func (l List) Every(step, size int) []Continuous {
	return box(Every(l, step, size))
}
