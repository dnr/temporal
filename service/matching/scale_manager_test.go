package matching

import (
	"testing"

	"github.com/stretchr/testify/assert"
	persistencespb "go.temporal.io/server/api/persistence/v1"
)

func TestBacklogStateBits(t *testing.T) {
	state := &persistencespb.PartitionScaleState{}

	// setting and reading individual bits
	assert.False(t, getBacklogStateBit(state, 0))

	setBacklogStateBit(state, 0)
	assert.Equal(t, []uint64{1}, state.BacklogState)
	assert.Equal(t, int32(1), readPartitionsFromBacklogState(state))
	assert.True(t, getBacklogStateBit(state, 0))
	assert.False(t, getBacklogStateBit(state, 1))

	setBacklogStateBit(state, 5)
	assert.Equal(t, []uint64{0b100001}, state.BacklogState)
	assert.Equal(t, int32(6), readPartitionsFromBacklogState(state))
	assert.True(t, getBacklogStateBit(state, 5))
	assert.False(t, getBacklogStateBit(state, 4))

	// bit in second word
	setBacklogStateBit(state, 64)
	assert.Equal(t, []uint64{0b100001, 1}, state.BacklogState)
	assert.Equal(t, int32(65), readPartitionsFromBacklogState(state))
	assert.True(t, getBacklogStateBit(state, 64))
	assert.False(t, getBacklogStateBit(state, 65))

	// bit at word boundary
	setBacklogStateBit(state, 63)
	assert.Equal(t, []uint64{0b100001 | (1 << 63), 1}, state.BacklogState)
	assert.Equal(t, int32(65), readPartitionsFromBacklogState(state))

	// clear high bit, read should drop back
	clearBacklogStateBit(state, 64)
	assert.Equal(t, []uint64{0b100001 | (1 << 63)}, state.BacklogState) // trailing zero word trimmed
	assert.Equal(t, int32(64), readPartitionsFromBacklogState(state))
	assert.False(t, getBacklogStateBit(state, 64))

	// clear all bits one by one
	clearBacklogStateBit(state, 63)
	clearBacklogStateBit(state, 5)
	clearBacklogStateBit(state, 0)
	assert.Empty(t, state.BacklogState)
	assert.Equal(t, int32(0), readPartitionsFromBacklogState(state))
	assert.False(t, getBacklogStateBit(state, 0))

	// clearing a bit that's already clear or out of range is a no-op
	clearBacklogStateBit(state, 999)
	assert.Empty(t, state.BacklogState)

	// read on nil returns 0
	assert.Equal(t, int32(0), readPartitionsFromBacklogState(nil))
}
