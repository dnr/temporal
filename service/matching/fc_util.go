package matching

import (
	"fmt"

	enumspb "go.temporal.io/api/enums/v1"
)

func wholeQueueLimiterName(tqName string, tqType enumspb.TaskQueueType) string {
	// the "/0" at the end is for future extension for partitioning limiters
	return fmt.Sprintf("/_sys/wholequeue/%s/%d/0", tqName, tqType)
}
