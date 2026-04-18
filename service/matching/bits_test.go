package matching

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBitSet(t *testing.T) {
	var bs bitSet

	// setting and reading individual bits
	assert.False(t, bs.get(0))

	bs.set(0)
	assert.Equal(t, bitSet{1}, bs)
	assert.Equal(t, int32(1), bs.len())
	assert.True(t, bs.get(0))
	assert.False(t, bs.get(1))

	bs.set(5)
	assert.Equal(t, bitSet{0b100001}, bs)
	assert.Equal(t, int32(6), bs.len())
	assert.True(t, bs.get(5))
	assert.False(t, bs.get(4))

	// bit in second word
	bs.set(64)
	assert.Equal(t, bitSet{0b100001, 1}, bs)
	assert.Equal(t, int32(65), bs.len())
	assert.True(t, bs.get(64))
	assert.False(t, bs.get(65))

	// bit at word boundary
	bs.set(63)
	assert.Equal(t, bitSet{0b100001 | (1 << 63), 1}, bs)
	assert.Equal(t, int32(65), bs.len())

	// clear high bit, read should drop back
	bs.clear(64)
	assert.Equal(t, bitSet{0b100001 | (1 << 63)}, bs) // trailing zero word trimmed
	assert.Equal(t, int32(64), bs.len())
	assert.False(t, bs.get(64))

	// clear all bits one by one
	bs.clear(63)
	bs.clear(5)
	bs.clear(0)
	assert.Empty(t, bs)
	assert.Equal(t, int32(0), bs.len())
	assert.False(t, bs.get(0))

	// clearing a bit that's already clear or out of range is a no-op
	bs.clear(999)
	assert.Empty(t, bs)

	// read on nil returns 0
	var nilbs bitSet
	assert.Equal(t, int32(0), nilbs.len())
}
