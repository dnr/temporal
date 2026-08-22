package stream_batcher

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/clock"
)

func TestKeyedBatcherReusesBatcherForKey(t *testing.T) {
	timeSource := clock.NewEventTimeSource()
	batcher := NewKeyedBatcher(
		func(string, []int) int { return 0 },
		BatcherOptions{},
		time.Hour,
		timeSource,
	)

	require.Same(t, batcher.get("a"), batcher.get("a"))
	require.NotSame(t, batcher.get("a"), batcher.get("b"))
}

func TestKeyedBatcherClearsBatchers(t *testing.T) {
	timeSource := clock.NewEventTimeSource()
	batcher := NewKeyedBatcher(
		func(string, []int) int { return 0 },
		BatcherOptions{},
		time.Hour,
		timeSource,
	)
	first := batcher.get("a")

	timeSource.Advance(time.Hour)
	batcher.maybeClear()

	require.NotSame(t, first, batcher.get("a"))
}
