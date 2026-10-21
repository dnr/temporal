package matching

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/testing/protorequire"
	"go.temporal.io/server/common/tqid"
	"go.temporal.io/server/service/matching/fc"
	"go.temporal.io/server/service/matching/simplelimiter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestPollForwardersBypassFlowControl(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind pollForwarderType
	}{
		{name: "parent", kind: parentPollForwarder},
		{name: "priority backlog", kind: priorityBacklogPollForwarder},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dc := dynamicconfig.NewNoopCollection()
			taskQueue := tqid.UnsafeTaskQueueFamily("ns", "tq").TaskQueue(enumspb.TASK_QUEUE_TYPE_WORKFLOW)
			cfg := newTaskQueueConfig(taskQueue, NewConfig(dc), "namespace")
			ts := clock.NewEventTimeSource().Update(time.Now())
			readiness := fc.NewReadiness(ts, nil, fc.NewReadinessConfig(dc))
			t.Cleanup(readiness.Stop)
			userData := &mockUserDataManager{}
			rateLimit := newRateLimitManager(userData, cfg, enumspb.TASK_QUEUE_TYPE_WORKFLOW)
			rateLimit.Start()
			t.Cleanup(rateLimit.Stop)
			rateLimit.SetEffectiveRPSAndSourceForTesting(0, enumspb.RATE_LIMIT_SOURCE_API)
			rateLimit.UpdateSimpleRateLimitWithBurstForTesting(0)
			rateLimit.SetFairnessKeyRateLimitDefaultForTesting(0)
			manager := newFCManager(taskQueue.RootPartition(), userData, rateLimit, readiness)
			data := newMatcherData(cfg, log.NewNoopLogger(), ts, true, manager, func(syncMatchOutcome) {})
			t.Cleanup(data.Stop)
			task := newPollForwarderTask(pollForwarderPriority, tc.kind)
			task.limiters = []fc.Limiter{{
				Key: "existing", Type: enumsspb.LIMITER_TYPE_CONCURRENCY, Source: fc.LimiterSourceConfig,
			}}
			require.NoError(t, data.EnqueueTaskNoWait(task))
			require.Nil(t, task.limiters)
			poller := &waitingPoller{startTime: ts.Now(), forwardCtx: context.Background()}
			require.Nil(t, data.MatchPollerImmediately(poller))
			require.NotNil(t, task.matchResult)
			require.Same(t, poller, task.matchResult.poller)
			require.Nil(t, task.fcTx)
		})
	}
}

type trailerCapturingStream struct {
	grpc.ServerTransportStream
	trailer metadata.MD
}

func (s *trailerCapturingStream) SetTrailer(md metadata.MD) error {
	s.trailer = metadata.Join(s.trailer, md)
	return nil
}

func TestLimiterHintsTrailer_RoundTrip(t *testing.T) {
	t.Parallel()
	stream := &trailerCapturingStream{}
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), stream)

	hints := []*taskqueuespb.LimiterHint{{
		LimiterType:        enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:                "key",
		Generation:         3,
		AvailableSlotsHint: 7,
	}}
	require.NoError(t, setLimiterHintsTrailer(ctx, hints))

	got := getLimiterHintsFromTrailer(stream.trailer)
	require.Len(t, got, 1)
	protorequire.ProtoEqual(t, hints[0], got[0])
}

func TestLimiterHintsTrailer_NoHints(t *testing.T) {
	t.Parallel()
	stream := &trailerCapturingStream{}
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), stream)

	require.NoError(t, setLimiterHintsTrailer(ctx, nil))
	require.Nil(t, stream.trailer)
	require.Nil(t, getLimiterHintsFromTrailer(nil))
}

func TestLimiterHintsTrailer_Malformed(t *testing.T) {
	t.Parallel()
	require.Nil(t, getLimiterHintsFromTrailer(metadata.Pairs(limiterHintsTrailerName, "\xff\xff")))
}

func TestFCManagerInitialGrants(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		rate    float64
		granted int32
	}{
		{name: "zero rate", rate: 0, granted: 0},
		{name: "one per second", rate: 1, granted: 2},
		{name: "ten per second", rate: 10, granted: 11},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			configClient := dynamicconfig.NewMemoryClient()
			configClient.OverrideSetting(dynamicconfig.AdminMatchingNamespaceTaskqueueToPartitionDispatchRate, test.rate)
			config := newTaskQueueConfig(
				tqid.UnsafeTaskQueueFamily("test-namespace", "test-task-queue").TaskQueue(enumspb.TASK_QUEUE_TYPE_ACTIVITY),
				NewConfig(dynamicconfig.NewCollection(configClient, log.NewNoopLogger())),
				"test-namespace",
			)
			manager := newRateLimitManager(nil, config, enumspb.TASK_QUEUE_TYPE_ACTIVITY)
			manager.Start()
			t.Cleanup(manager.Stop)
			fcManager := newGrantTestFCManager(t, manager, clock.NewEventTimeSource().Update(time.Now()))

			require.Equal(t, test.granted, fcManager.grantEagerTokens(nil, 20))
			require.Zero(t, fcManager.grantEagerTokens(nil, 1))
		})
	}
}

func TestFCManagerGrantTokens(t *testing.T) {
	t.Run("new matcher applies the whole queue limit", func(t *testing.T) {
		manager := &rateLimitManager{
			config:          &taskQueueConfig{},
			wholeQueueLimit: new(simplelimiter.MakeParams(2, 500*time.Millisecond)),
		}

		fcManager := newGrantTestFCManager(t, manager, clock.NewEventTimeSource().Update(time.Now()))
		granted := fcManager.grantEagerTokens(nil, 5)
		require.Equal(t, int32(2), granted)
		granted = fcManager.grantEagerTokens(nil, 1)
		require.Equal(t, int32(0), granted)
	})

	t.Run("new matcher isolates fairness keys", func(t *testing.T) {
		timeSource := clock.NewEventTimeSource().Update(time.Now())
		manager := &rateLimitManager{
			config:          &taskQueueConfig{},
			wholeQueueLimit: new(simplelimiter.NoLimit()),
			perKeyLimit:     simplelimiter.MakeParams(1, 0),
		}

		keyOne := &commonpb.Priority{FairnessKey: "one"}
		keyTwo := &commonpb.Priority{FairnessKey: "two"}
		fcManager := newGrantTestFCManager(t, manager, timeSource)
		granted := fcManager.grantEagerTokens(keyOne, 20)
		require.Equal(t, int32(1), granted)
		granted = fcManager.grantEagerTokens(keyOne, 1)
		require.Equal(t, int32(0), granted)
		granted = fcManager.grantEagerTokens(keyTwo, 1)
		require.Equal(t, int32(1), granted)

		timeSource.Advance(time.Second)
		granted = fcManager.grantEagerTokens(keyOne, 2)
		require.Equal(t, int32(1), granted)
	})
}

func newGrantTestFCManager(t *testing.T, rateLimit *rateLimitManager, timeSource clock.TimeSource) *fcManager {
	t.Helper()
	partition := tqid.UnsafeTaskQueueFamily("test-namespace", "test-task-queue").TaskQueue(enumspb.TASK_QUEUE_TYPE_ACTIVITY).RootPartition()
	readiness := fc.NewReadiness(timeSource, nil, fc.NewReadinessConfig(dynamicconfig.NewNoopCollection()))
	t.Cleanup(readiness.Stop)
	return newFCManager(partition, &mockUserDataManager{}, rateLimit, readiness)
}

type eagerTestCallback struct{}

func (*eagerTestCallback) OnReady() {}

func TestEagerDispatchSharesRateTokens(t *testing.T) {
	t.Parallel()
	for _, eagerFirst := range []bool{false, true} {
		name := "regular first"
		if eagerFirst {
			name = "eager first"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rateLimit := &rateLimitManager{
				wholeQueueLimit: new(simplelimiter.MakeParams(1, 0)),
			}
			manager := newGrantTestFCManager(t, rateLimit, clock.NewEventTimeSource().Update(time.Now()))
			task := newInternalTaskForSyncMatch(&persistencespb.TaskInfo{CreateTime: timestamppb.Now()}, nil, 0, nil)
			var err error
			task.limiters, err = manager.UpdateLimitersFromConfig(nil, task)
			require.NoError(t, err)
			cb := &eagerTestCallback{}
			t.Cleanup(func() { manager.CancelAllCallbacks(cb) })
			if eagerFirst {
				require.Equal(t, int32(1), manager.grantEagerTokens(nil, 1))
				require.ErrorIs(t, manager.TaskReady(task, cb), fc.ErrLocalLimiterBlocked)
			} else {
				require.NoError(t, manager.TaskReady(task, cb))
				task.fcTx.Close()
				require.Zero(t, manager.grantEagerTokens(nil, 1))
			}
		})
	}
}

func TestEagerDispatchBlocksConcurrencyLimitedQueues(t *testing.T) {
	t.Parallel()
	manager := newGrantTestFCManager(t, &rateLimitManager{
		wholeQueueLimit: new(simplelimiter.MakeParams(1, 0)),
	}, clock.NewEventTimeSource().Update(time.Now()))
	userData := &mockUserDataManager{}
	manager.userDataManager = userData
	userData.data = &persistencespb.VersionedTaskQueueUserData{Data: &persistencespb.TaskQueueUserData{
		PerType: map[int32]*persistencespb.TaskQueueTypeUserData{
			int32(enumspb.TASK_QUEUE_TYPE_ACTIVITY): {Config: &taskqueuepb.TaskQueueConfig{
				QueueConcurrencyLimit: &taskqueuepb.ConcurrencyLimitConfig{ConcurrencyLimit: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 10}},
			}},
		},
	}}
	require.Zero(t, manager.grantEagerTokens(nil, 1))
	userData.data = nil
	require.Equal(t, int32(1), manager.grantEagerTokens(nil, 1), "blocked concurrency grants must not consume rate tokens")
}

func TestEagerDispatchFairnessGrantPreservesOtherKeysCapacity(t *testing.T) {
	t.Parallel()
	manager := newGrantTestFCManager(t, &rateLimitManager{
		wholeQueueLimit: new(simplelimiter.MakeParams(2, 500*time.Millisecond)),
		perKeyLimit:     simplelimiter.MakeParams(1, 0),
	}, clock.NewEventTimeSource().Update(time.Now()))
	first := &commonpb.Priority{FairnessKey: "first"}
	second := &commonpb.Priority{FairnessKey: "second"}
	require.Equal(t, int32(1), manager.grantEagerTokens(first, 10))
	require.Zero(t, manager.grantEagerTokens(first, 1))
	require.Equal(t, int32(1), manager.grantEagerTokens(second, 10))
	require.Zero(t, manager.grantEagerTokens(second, 1))
}

func TestEagerDispatchConcurrentGrants(t *testing.T) {
	t.Parallel()
	manager := newGrantTestFCManager(t, &rateLimitManager{
		wholeQueueLimit: new(simplelimiter.MakeParams(10, time.Second)),
	}, clock.NewEventTimeSource().Update(time.Now()))
	const callers = 16
	results := make(chan int32, callers)
	for range callers {
		go func() { results <- manager.grantEagerTokens(nil, 5) }()
	}
	var total int32
	for range callers {
		total += <-results
	}
	require.Equal(t, int32(11), total)
}
