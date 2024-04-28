package sdk

/*
Compare contents of 2 slices of bytes
*/
func SameBytes(left, right []byte) bool {
	if len(left) == 0 {
		return len(right) == 0
	}

	if right == nil || len(left) != len(right) {
		return false
	}

	for index, sample := range left {
		if sample != right[index] {
			return false
		}
	}

	return true
}
