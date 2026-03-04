package matching

import "math/bits"

func readPartitionsFromBacklogState(state []uint64) int32 {
	i := len(state) - 1
	if i < 0 {
		return 0
	}
	return int32(bits.Len64(state[i]) - 1 + i*64)
}
