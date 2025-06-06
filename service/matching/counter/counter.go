package counter

type Counter interface {
	GetPass(key string, base, inc int64) int64
}
