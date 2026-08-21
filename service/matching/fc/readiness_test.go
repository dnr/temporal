package fc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	enumsspb "go.temporal.io/server/api/enums/v1"
	"go.temporal.io/server/common/namespace"
)

type testFCTask struct {
	limiters *Limiters
}

func (t testFCTask) Limiters() *Limiters {
	return t.limiters
}

func TestNewTxGeneratesSlotIDs(t *testing.T) {
	limiters := &Limiters{}
	limiters.limiters[0] = limiter{
		key:    "limiter-b",
		tp:     enumsspb.LIMITER_TYPE_CONCURRENCY,
		source: limiterSourceTask,
	}
	limiters.limiters[1] = limiter{
		key:    "limiter-a",
		tp:     enumsspb.LIMITER_TYPE_CONCURRENCY,
		source: limiterSourceTask,
	}

	tx, err := NewReadiness(nil).NewTx(
		context.Background(),
		namespace.ID("namespace-id"),
		testFCTask{limiters: limiters},
	)
	require.NoError(t, err)
	require.Len(t, tx.LimiterRefs(), 2)

	firstRef := tx.LimiterRefs()[0]
	secondRef := tx.LimiterRefs()[1]
	require.Equal(t, "limiter-a", firstRef.GetKey())
	require.Equal(t, "limiter-b", secondRef.GetKey())
	require.NoError(t, uuid.Validate(firstRef.GetSlotId()))
	require.NoError(t, uuid.Validate(secondRef.GetSlotId()))
	require.NotEqual(t, firstRef.GetSlotId(), secondRef.GetSlotId())
	require.Equal(t, firstRef.GetSlotId(), tx.committers[0].(*concurrencyCommitter).slotID)
	require.Equal(t, secondRef.GetSlotId(), tx.committers[1].(*concurrencyCommitter).slotID)

	nextTx, err := NewReadiness(nil).NewTx(
		context.Background(),
		namespace.ID("namespace-id"),
		testFCTask{limiters: limiters},
	)
	require.NoError(t, err)
	require.NotEqual(t, firstRef.GetSlotId(), nextTx.LimiterRefs()[0].GetSlotId())
}
