package matching

import enumspb "go.temporal.io/api/enums/v1"

// Maximum total limiters that can apply to any task. (Currently this includes the whole-queue
// limiter, in the future we may allow the whole-queue limiter to be separate from this limit.)
const maxLimiters = 3

type limiterSource int32

const (
	// Limiter came from task queue config
	limiterSourceTQConfig limiterSource = iota
	// Limiter came from task itself
	limiterSourceTask
	// Future: namespace policy, etc.
)

type fcLimiter struct {
	key    string
	source limiterSource
	// isWholeQueue bool // FIXME: do we need this?
}

type fcLimiters struct {
	limiters [maxLimiters]*fcLimiter
}

type fcManager struct {
	userDataManager userDataManager
	tqType          enumspb.TaskQueueType
	// FIXME: need client for external limiters
	// FIXME: need some way to call back into matcher when readiness changes
}

func newFlowControlManager(
	userDataManager userDataManager,
	tqType enumspb.TaskQueueType,
) *fcManager {
	return &fcManager{
		userDataManager: userDataManager,
		tqType:          tqType,
	}
}

func (fc *fcManager) WholeQueueLikely() bool {
	// FIXME
	return true
}
