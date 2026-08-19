package matching

import (
	"errors"
	"slices"

	"go.temporal.io/server/common/tqid"
)

// Maximum total limiters that can apply to any task. (Currently this includes the whole-queue
// limiter, in the future we may allow the whole-queue limiter to be separate from this limit.)
const maxLimiters = 3

type limiterType int32

const (
	// not valid limiter
	limiterTypeInvalid limiterType = iota
	// concurrency limiter
	limiterTypeConcurrency
	// future: rate limit, circuit breaker, etc.
)

type limiterSource int32

const (
	// not valid limiter
	limiterSourceInvalid limiterSource = iota
	// limiter came from task queue config, applies to the whole queue
	limiterSourceConfig_WholeQueue
	// limiter came from task itself
	limiterSourceTask
	// future: namespace policy, etc.
)

type fcLimiter struct {
	key    string // TODO(fc): consider interning? or intern whole fcLimiter?
	tp     limiterType
	source limiterSource
}

func (lim fcLimiter) valid() bool {
	return lim.tp != limiterTypeInvalid && lim.source != limiterSourceInvalid
}

type fcLimiters struct {
	limiters [maxLimiters]fcLimiter
}

type fcManager struct {
	partition       tqid.Partition
	userDataManager userDataManager
	// FIXME: need client for external limiters
	// FIXME: need some way to call back into matcher when readiness changes
}

func newFCManager(
	partition tqid.Partition,
	userDataManager userDataManager,
) *fcManager {
	return &fcManager{
		partition:       partition,
		userDataManager: userDataManager,
	}
}

func (fc *fcManager) WholeQueueLikely() bool {
	// FIXME
	return true
}

func (fc *fcManager) UpdateLimitersFromConfig(
	task *internalTask,
) error {
	userData, _, err := fc.userDataManager.GetUserData()
	if err != nil {
		return err
	}
	tqName := fc.partition.TaskQueue().Name()
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
				key:    wholeQueueLimiterName(tqName, tqType),
				tp:     limiterTypeConcurrency,
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
