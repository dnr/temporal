package matching

import "math/bits"

type bitSet []uint64

func (bs bitSet) len() int32 {
	i := len(bs) - 1
	if i < 0 {
		return 0
	}
	return int32(bits.Len64(bs[i]) + i*64)
}

func (bs bitSet) get(i int32) bool {
	if len(bs) < int(i)/64+1 {
		return false
	}
	return bs[i/64]&(1<<(i%64)) != 0
}

func (bs *bitSet) set(i int32) {
	for len(*bs) < int(i)/64+1 {
		*bs = append(*bs, 0)
	}
	(*bs)[i/64] |= 1 << (i % 64)
}

func (bs *bitSet) clear(i int32) {
	if len(*bs) < int(i)/64+1 {
		return
	}
	(*bs)[i/64] &^= 1 << (i % 64)
	for len(*bs) > 0 && (*bs)[len(*bs)-1] == 0 {
		*bs = (*bs)[:len(*bs)-1]
	}
}
