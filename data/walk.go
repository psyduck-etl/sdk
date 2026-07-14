package data

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/itchyny/gojq"
)

// Path is a sequence of segments walked across mixed continuous/discrete data.
// A segment that parses as an integer indexes a continuous node; otherwise it
// is a discrete key.
type Path []string

// Walk descends a Value along a discrete/continuous path. At each segment: if
// the node is Discrete, the segment is a key; if the node is Continuous, the
// segment is parsed as an int index. This is exactly the model's rule —
// continuous is referenced by index, discrete by key. It backs the `path`
// selector. ok=false means the path did not resolve.
func Walk(v Value, p Path) (Value, bool, error) {
	cur := v
	for _, seg := range p {
		switch node := cur.(type) {
		case Discrete:
			next, ok := node.Get(seg)
			if !ok {
				return nil, false, nil
			}
			cur = next
		case Continuous:
			n, err := strconv.Atoi(seg)
			if err != nil {
				return nil, false, fmt.Errorf("segment %q is not an index into a %s", seg, cur.Kind())
			}
			win, ok := node.Get(n)
			if !ok {
				return nil, false, nil
			}
			// Get returns a length-1 window; unwrap list elements to the value.
			if l, isList := win.(List); isList && len(l) == 1 {
				cur = l[0]
			} else {
				cur = win
			}
		default:
			// Lit or other leaf: cannot descend further.
			return nil, false, nil
		}
	}
	return cur, true, nil
}

// ByJQ evaluates a jq expression against a value and returns the first result
// as a Value. It backs the `by` selector for continuous/linear data. A query
// that yields no output returns ok=false.
func ByJQ(v Value, expr string) (Value, bool, error) {
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, false, fmt.Errorf("jq: parse %q: %w", expr, err)
	}
	return runByJQ(query, v)
}

// CompileJQ pre-parses a jq expression so a transformer can compile once and
// evaluate per message.
func CompileJQ(expr string) (*gojq.Query, error) {
	return gojq.Parse(expr)
}

// EvalJQ evaluates a pre-parsed jq query against a value.
func EvalJQ(query *gojq.Query, v Value) (Value, bool, error) {
	return runByJQ(query, v)
}

func runByJQ(query *gojq.Query, v Value) (Value, bool, error) {
	input := Native(v)
	// gojq wants json.Number-free float64 input for arithmetic; re-normalize.
	input = normalizeForJQ(input)

	iter := query.Run(input)
	out, ok := iter.Next()
	if !ok {
		return nil, false, nil
	}
	if err, isErr := out.(error); isErr {
		return nil, false, fmt.Errorf("jq: %w", err)
	}
	return fromNative(out), true, nil
}

// normalizeForJQ converts json.Number values to float64 so gojq can operate on
// them, recursing through maps and slices.
func normalizeForJQ(n any) any {
	switch t := n.(type) {
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	case []any:
		for i, e := range t {
			t[i] = normalizeForJQ(e)
		}
		return t
	case map[string]any:
		for k, e := range t {
			t[k] = normalizeForJQ(e)
		}
		return t
	default:
		return n
	}
}

// AtoiCoerce is a ready-made coercer for TryGet on int-keyed containers.
func AtoiCoerce(s string) (int, error) { return strconv.Atoi(s) }

// StringCoerce is the identity coercer for string-keyed containers.
func StringCoerce(s string) (string, error) { return s, nil }
