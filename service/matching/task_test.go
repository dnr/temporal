package matching

import (
	"context"
	"errors"
	"testing"
	"time"

	enumsspb "go.temporal.io/server/api/enums/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/service/matching/fc"
	"go.temporal.io/server/service/matching/simplelimiter"

	"github.com/stretchr/testify/require"
)

func TestTaskFinishResult(t *testing.T) {
	t.Parallel()

	// success
	task, tokenAvailable := newTaskWithRateLimitToken(t)
	task.finish(nil)
	got, ok := task.getResponse()
	require.True(t, ok)
	require.Equal(t, taskResponse{}, got)
	require.False(t, tokenAvailable())

	// start error
	task, tokenAvailable = newTaskWithRateLimitToken(t)
	startErr := errors.New("start failed")
	task.finish(startErr)
	got, ok = task.getResponse()
	require.True(t, ok)
	require.Equal(t, taskResponse{startErr: startErr}, got)
	require.True(t, tokenAvailable())

	// drop
	task, tokenAvailable = newTaskWithRateLimitToken(t)
	task.finish(dropReasonInvalid)
	got, ok = task.getResponse()
	require.True(t, ok)
	require.Equal(t, taskResponse{dropReason: dropReasonInvalid}, got)
	require.True(t, tokenAvailable())

	// forward success
	task, tokenAvailable = newTaskWithRateLimitToken(t)
	task.finishForward("response", nil)
	got, ok = task.getResponse()
	require.True(t, ok)
	require.Equal(t, taskResponse{forwarded: true, forwardRes: "response"}, got)
	require.False(t, tokenAvailable())
	task.fcTx.Rollback(t.Context())
	require.False(t, tokenAvailable())

	// forward error
	task, tokenAvailable = newTaskWithRateLimitToken(t)
	forwardErr := errors.New("forward failed")
	task.finishForward(nil, forwardErr)
	got, ok = task.getResponse()
	require.True(t, ok)
	require.Equal(t, taskResponse{forwarded: true, forwardErr: forwardErr}, got)
	require.True(t, tokenAvailable())
}

func newTaskWithRateLimitToken(t *testing.T) (*internalTask, func() bool) {
	t.Helper()
	readiness := fc.NewReadiness(clock.NewEventTimeSource().Update(time.Now()), nil, fc.NewReadinessConfig(dynamicconfig.NewNoopCollection()))
	t.Cleanup(readiness.Stop)
	task := &internalTask{
		responseC: make(chan taskResponse, 1),
		limiters: []fc.Limiter{{
			Type:   enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT,
			Key:    "limiter",
			Config: &simplelimiter.Params{Interval: time.Second},
		}},
	}
	task.fcTx = readiness.NewTx("namespace", task, nil)
	require.NoError(t, task.fcTx.Check())
	return task, func() bool {
		tx := readiness.NewTx("namespace", task, nil)
		defer tx.Rollback(context.Background())
		return tx.Check() == nil
	}
}

func TestTaskFinishForwardQueryErrorRetainsToken(t *testing.T) {
	t.Parallel()
	task, tokenAvailable := newTaskWithRateLimitToken(t)
	task.query = &queryTaskInfo{}

	task.finishForward(nil, errors.New("query failed"))

	require.False(t, tokenAvailable())
}

func TestTaskFinishForwardNexusErrorRetainsToken(t *testing.T) {
	t.Parallel()
	task, tokenAvailable := newTaskWithRateLimitToken(t)
	task.nexus = &nexusTaskInfo{}

	task.finishForward(nil, errors.New("nexus failed"))

	require.False(t, tokenAvailable())
}
