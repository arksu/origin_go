package mathutil

// FloorDiv returns mathematical floor(a / b) for integers, including negative coordinates.
// Go's integer division truncates toward zero, which is wrong for chunk/tile mapping with negatives.
func FloorDiv(a, b int) int {
	if b == 0 {
		return 0
	}
	q := a / b
	r := a % b
	if r != 0 && ((r < 0) != (b < 0)) {
		q--
	}
	return q
}
