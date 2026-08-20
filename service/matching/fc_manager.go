package matching

import (
	"errors"
	"slices"

	enumsspb "go.temporal.io/server/api/enums/v1"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/tqid"
)

type fcLimiter struct {
	key    string // TODO(fc): consider interning? or intern whole fcLimiter?
	tp     enumsspb.LimiterType
	source limiterSource
}

func (lim fcLimiter) valid() bool {
	return lim.tp != enumsspb.LIMITER_TYPE_UNSPECIFIED && lim.source != limiterSourceInvalid
}

type fcLimiters struct {
	limiters [maxLimiters]fcLimiter
}

type fcManager struct {
	partition         tqid.Partition
	userDataManager   userDataManager
	readiness         *fcReadiness
	wholeQueueLimiter string
	// FIXME: need some way to call back into matcher when readiness changes
}

func newFCManager(
	partition tqid.Partition,
	userDataManager userDataManager,
	readiness *fcReadiness,
) *fcManager {
	tqName := partition.TaskQueue().Name()
	tqType := partition.TaskType()
	return &fcManager{
		partition:         partition,
		userDataManager:   userDataManager,
		readiness:         readiness,
		wholeQueueLimiter: wholeQueueLimiterName(tqName, tqType),
	}
}

func (fc *fcManager) WholeQueueLikely() bool {
	nsID := namespace.ID(fc.partition.NamespaceId())
	state := fc.readiness.ReadyState(nsID, fc.wholeQueueLimiter)
	return state == fcReadinessUnknown || state == fcReadinessReady
}

func (fc *fcManager) UpdateLimitersFromConfig(
	task *internalTask,
) error {
	userData, _, err := fc.userDataManager.GetUserData()
	if err != nil {
		return err
	}
	tqType := fc.partition.TaskType()
	limit := userData.GetData().GetPerType()[int32(tqType)].GetConfig().GetQueueConcurrencyLimit().GetConcurrencyLimit()
	if limit == nil {
		// there is no limit. clear if it was set already.
		if task.limiters != nil {
			_ = slices.DeleteFunc(task.limiters.limiters[:], func(lim fcLimiter) bool {
				return lim.source == limiterSourceConfig_WholeQueue
			})
		}
		return nil
	}
	// need to add limiter
	if task.limiters == nil {
		task.limiters = &fcLimiters{}
	}
	for i, lim := range task.limiters.limiters[:] {
		if !lim.valid() {
			// add it in empty slot
			task.limiters.limiters[i] = fcLimiter{
				key:    fc.wholeQueueLimiter,
				tp:     enumsspb.LIMITER_TYPE_CONCURRENCY,
				source: limiterSourceConfig_WholeQueue,
			}
			return nil
		} else if lim.source == limiterSourceConfig_WholeQueue {
			// we found one we previously set
			return nil
		}
	}
	return errors.New("too many limiters") // FIXME: proper type
}
