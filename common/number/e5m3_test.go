package number

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type e5m3Suite struct {
	suite.Suite
	*require.Assertions
}

func TestE5M3Suite(t *testing.T) {
	suite.Run(t, new(e5m3Suite))
}

func (s *e5m3Suite) SetupTest() {
	s.Assertions = require.New(s.T())
}

func (s *e5m3Suite) TestDecodeKnownValues() {
	s.Equal(int64(0), DecodeE5M3(0))
	// Subnormals: E=0, M << 6
	s.Equal(int64(64), DecodeE5M3(1))    // 1 << 6
	s.Equal(int64(128), DecodeE5M3(2))   // 2 << 6
	s.Equal(int64(448), DecodeE5M3(7))   // 7 << 6
	// E=1: (8|M) << 6
	s.Equal(int64(512), DecodeE5M3(8))   // 8 << 6
	s.Equal(int64(960), DecodeE5M3(15))  // 15 << 6
	// E=2: (8|M) << 7
	s.Equal(int64(1024), DecodeE5M3(16)) // 8 << 7
	s.Equal(int64(1920), DecodeE5M3(23)) // 15 << 7
	// Max: E=31, M=7: 15 << 36 ≈ 1.03 trillion
	s.Equal(int64(15)<<36, DecodeE5M3(255))
}

func (s *e5m3Suite) TestEncodeKnownValues() {
	s.Equal(E5M3(0), EncodeE5M3(0))
	s.Equal(E5M3(0), EncodeE5M3(-1))
	s.Equal(E5M3(0), EncodeE5M3(63))    // below smallest subnormal
	s.Equal(E5M3(1), EncodeE5M3(64))    // smallest subnormal
	s.Equal(E5M3(1), EncodeE5M3(100))   // rounds down to 64
	s.Equal(E5M3(3), EncodeE5M3(200))   // rounds down to 192
	s.Equal(E5M3(8), EncodeE5M3(512))
	s.Equal(E5M3(16), EncodeE5M3(1024))
	s.Equal(E5M3(255), EncodeE5M3(1e15))
}

func (s *e5m3Suite) TestEncodeRoundsDown() {
	// 1000 is between 960 (code 15) and 1024 (code 16)
	s.Equal(E5M3(15), EncodeE5M3(1000))
	s.Equal(int64(960), DecodeE5M3(EncodeE5M3(1000)))
	// 1023 still rounds down to 960
	s.Equal(E5M3(15), EncodeE5M3(1023))
}

func (s *e5m3Suite) TestRoundtrip() {
	for b := 0; b < 256; b++ {
		decoded := DecodeE5M3(E5M3(b))
		reencoded := EncodeE5M3(decoded)
		s.Equalf(E5M3(b), reencoded, "byte=%d decoded=%d", b, decoded)
	}
}

func (s *e5m3Suite) TestMonotonic() {
	prev := int64(0)
	for b := 0; b < 256; b++ {
		v := DecodeE5M3(E5M3(b))
		s.GreaterOrEqualf(v, prev, "byte=%d", b)
		prev = v
	}
}

func (s *e5m3Suite) TestRoundDown() {
	for b := 0; b < 255; b++ {
		decoded := DecodeE5M3(E5M3(b))
		nextDecoded := DecodeE5M3(E5M3(b + 1))
		if nextDecoded <= decoded+1 {
			continue // consecutive integers, can't test rounding
		}
		mid := decoded + (nextDecoded-decoded)/2
		encoded := EncodeE5M3(mid)
		s.Equalf(E5M3(b), encoded, "value=%d should round down to byte=%d (not %d)", mid, b, encoded)
	}
}

func (s *e5m3Suite) TestUpdateSameBucket() {
	// Values within the same bucket don't trigger an update
	code := EncodeE5M3(1024) // E=2, M=0, covers [1024, 1152)
	s.Equal(code, UpdateE5M3(1024, code))
	s.Equal(code, UpdateE5M3(1100, code))
	s.Equal(code, UpdateE5M3(1151, code))
}

func (s *e5m3Suite) TestUpdateHysteresis() {
	code1024 := EncodeE5M3(1024) // E=2, M=0
	code960 := EncodeE5M3(960)   // E=1, M=7
	s.Equal(int64(1024), DecodeE5M3(code1024))
	s.Equal(int64(960), DecodeE5M3(code960))

	// gap=64, margin=32, so transition from code_1024 down at value<976.

	// From code_1024: small drops stay (hysteresis keeps it sticky)
	s.Equal(code1024, UpdateE5M3(1000, code1024))
	s.Equal(code1024, UpdateE5M3(980, code1024))
	s.Equal(code1024, UpdateE5M3(976, code1024)) // boundary: stays

	// From code_1024: large enough drop switches
	s.Equal(code960, UpdateE5M3(975, code1024))
	s.Equal(code960, UpdateE5M3(960, code1024))

	// From code_960: values below 1024 still encode to code_960, no change
	s.Equal(code960, UpdateE5M3(1023, code960))
	s.Equal(code960, UpdateE5M3(990, code960))

	// From code_960: value reaches next bucket, switches up
	s.Equal(code1024, UpdateE5M3(1024, code960))
}

func (s *e5m3Suite) TestUpdateLargeJump() {
	code := EncodeE5M3(960)
	result := UpdateE5M3(100000, code)
	s.Equal(EncodeE5M3(100000), result)
}

func (s *e5m3Suite) TestUpdateZeroBoundary() {
	s.Equal(EncodeE5M3(1024), UpdateE5M3(1024, 0))
	s.Equal(E5M3(0), UpdateE5M3(0, EncodeE5M3(1024)))
}
