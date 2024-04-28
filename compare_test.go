package sdk

import "testing"

func TestSameBytes(t *testing.T) {
	testcases := [...]struct {
		left, right []byte
		same        bool
	}{
		{[]byte("foo"), []byte("foo"), true},
		{[]byte("foo"), []byte("foo\n"), false},
		{[]byte("foo"), []byte("bar"), false},
		{nil, nil, true},
		{[]byte("foo"), nil, false},
		{nil, []byte("foo"), false},
		{make([]byte, 1), []byte("\000"), true},
		{[]byte{0}, []byte("\000"), true},
		{[]byte{}, []byte(""), true},
		{make([]byte, 0), []byte(""), true},
		{[]byte{}, make([]byte, 0), true},
		{[]byte(""), nil, true},
	}

	for _, tc := range testcases {
		if same := SameBytes(tc.left, tc.right); same != tc.same {
			t.Fatalf("%s %v == %s %v isn't %t", tc.left, tc.left, tc.right, tc.right, tc.same)
		}
	}
}
