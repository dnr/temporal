package matching

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	enumspb "go.temporal.io/api/enums/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/tqid"
	"go.temporal.io/server/service/matching/fc"
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
			rateLimit := newRateLimitManager(ts, userData, cfg, enumspb.TASK_QUEUE_TYPE_WORKFLOW)
			rateLimit.Start()
			t.Cleanup(rateLimit.Stop)
			rateLimit.SetEffectiveRPSAndSourceForTesting(0, enumspb.RATE_LIMIT_SOURCE_API)
			rateLimit.UpdateSimpleRateLimitWithBurstForTesting(0)
			rateLimit.SetFairnessKeyRateLimitDefaultForTesting(0)
			manager := newFCManager(taskQueue.RootPartition(), cfg, userData, rateLimit, readiness)
			data := newMatcherData(cfg, log.NewNoopLogger(), ts, true, rateLimit, manager, func(syncMatchOutcome) {})
			t.Cleanup(data.Stop)
			task := newPollForwarderTask(pollForwarderPriority, tc.kind)
			task.limiters = &fc.Limiters{Limiters: [fc.MaxLimiters]fc.Limiter{{
				Key: "existing", Type: enumsspb.LIMITER_TYPE_CONCURRENCY, Source: fc.LimiterSourceConfig,
			}}}
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
