package matching

import enumspb "go.temporal.io/api/enums/v1"

type fcManager struct {
	userDataManager userDataManager
	tqType          enumspb.TaskQueueType
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
	return true
}
