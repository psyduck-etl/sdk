package sdk

func SameBytes(left, right []byte) bool {
	if left == nil {
		return right == nil
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
