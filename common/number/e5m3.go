package number

import "math/bits"

// E5M3 is an unsigned 8-bit minifloat with 5 exponent bits and 3 mantissa bits.
// It represents non-negative integers with approximately 12.5% relative precision
// (8 distinct values per octave).
//
// Encoding (for a byte with top 5 bits E and bottom 3 bits M):
//
//	E=0, M=0:  0                        (zero)
//	E=0, M>0:  M << (offset+1)          (subnormal)
//	E>=1:      (8|M) << (E+offset)      (normalized)
type E5M3 = uint8

// The offset parameter controls the representable range.
// Using an offset of 5 allows representing values from 64 to ~1 trillion.
const e5m3offset = 5

// Decode converts an E5M3 value to an int64.
func DecodeE5M3(b E5M3) int64 {
	if b == 0 {
		return 0
	}
	e := int(b >> 3)
	m := int(b & 7)
	if e == 0 {
		return int64(m) << (e5m3offset + 1)
	}
	return int64(8|m) << (e + e5m3offset)
}

// EncodeE5M3 encodes a non-negative int64 into E5M3 representation.
// The value is rounded down to the nearest representable value.
// Negative values go to 0 and values above the maximum representable go to 255.
func EncodeE5M3(value int64) E5M3 {
	if value <= 0 {
		return 0
	}

	uval := uint64(value)
	bitLen := bits.Len64(uval)

	shift := bitLen - 4
	e := shift - e5m3offset

	if e > 31 {
		return 255
	}

	if e >= 1 {
		m := int(uval>>uint(shift)) & 7
		return E5M3(e<<3 | m)
	}

	// Subnormal: value = M << (offset+1), M ∈ [1,7]
	m := int(uval >> (e5m3offset + 1))
	if m < 1 {
		return 0
	}
	if m > 7 {
		m = 7
	}
	return E5M3(m)
}

// UpdateE5M3 returns the E5M3 encoding of value, but with hysteresis: it
// sticks to prev unless the new code is significantly closer. This prevents
// oscillation when the underlying value fluctuates near a bucket boundary.
func UpdateE5M3(value int64, prev E5M3) E5M3 {
	newCode := EncodeE5M3(value)
	if newCode == prev {
		return prev
	}
	newDist := value - DecodeE5M3(newCode) // always >= 0 (round-down)
	oldDist := DecodeE5M3(prev) - value
	if oldDist < 0 {
		oldDist = -oldDist
	}
	// Require the new code to be closer by at least half a bucket width
	// (at the smaller of the two exponent levels). This shifts the
	// transition point from the midpoint to the 3/4 mark of the gap,
	// creating a dead zone that prevents chatter.
	e := min(int(prev>>3), int(newCode>>3))
	margin := int64(1) << (e + e5m3offset - 1)
	if newDist < oldDist-margin {
		return newCode
	}
	return prev
}
