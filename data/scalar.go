package data

import (
	"encoding/json"
	"fmt"
)

// Lit is an atomic JSON scalar leaf — number, bool, or null — that preserves
// its native Go value so a json→Value→json round-trip is faithful (a number
// stays a number, not a quoted string). JSON strings use Str, not Lit.
//
// A Lit is a leaf: it is neither continuous (no index ops) nor discrete (no
// keys). Walking a Path into a Lit yields "not found".
type Lit struct{ V any }

func (l Lit) Kind() Kind { return KindLit }

func (l Lit) Bytes() []byte {
	if l.V == nil {
		return []byte("null")
	}
	b, err := json.Marshal(l.V)
	if err != nil {
		return []byte(sprint(l.V))
	}
	return b
}

func (l Lit) String() string { return string(l.Bytes()) }

func sprint(v any) string { return fmt.Sprintf("%v", v) }
