package matching

import (
	"errors"
	"fmt"

	enumspb "go.temporal.io/api/enums/v1"
)

func wholeQueueLimiterName(tqName string, tqType enumspb.TaskQueueType) string {
	// the "/0" at the end is for future extension for partitioning limiters
	return fmt.Sprintf("/_sys/wholequeue/%s/%d/0", tqName, tqType)
}

func addLimitersFromTQConfig(
	task *internalTask,
	userDataManager userDataManager,
	tqName string,
	tqType enumspb.TaskQueueType,
) error {
	userData, _, err := userDataManager.GetUserData()
	if err != nil {
		return err
	}
	limit := userData.GetData().GetPerType()[tqType].GetConfig().GetQueueConcurrencyLimit().GetConcurrencyLimit()
	if limit == nil {
		return nil
	}
	// need to add limiter
	if task.limiters == nil {
		task.limiters = &fcLimiters{}
	}
	for i, lim := range task.limiters.limiters[:] {
		if lim == nil {
			task.limiters.limiters[i] = &fcLimiter{
				key:    wholeQueueLimiterName(tqName, tqType),
				source: limiterSourceTQConfig,
			}
			return nil
		}
	}
	return errors.New("too many limiters") // FIXME: proper type
}
