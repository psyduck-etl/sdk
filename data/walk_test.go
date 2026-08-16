package data

import (
	"encoding/json"
	"testing"
)

// normalizeForJQ's json.Number branch falls back to the raw string when
// Float64() errors, but nothing forced that error before: every number in
// the existing walk/jq tests parses cleanly. "1e400" is syntactically a
// valid JSON number but overflows float64 (strconv.ErrRange), so it
// exercises the fallback.
func TestNormalizeForJQNumberOverflow(t *testing.T) {
	got := normalizeForJQ(json.Number("1e400"))
	if got != "1e400" {
		t.Errorf("normalizeForJQ(overflow number) = %v, want the raw string %q", got, "1e400")
	}
}
