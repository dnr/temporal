package taskqueue

import (
	"bytes"
	"encoding/binary"

	"github.com/google/uuid"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
)

func TaskUUID(
	workflowID string,
	runID string,
	scheduledEventID int64,
	componentRef []byte,
	stamp int32,
) string {
	// FIXME: Make workflow-backed activity retry stamp increments unconditional before relying on
	// TaskUUID to identify each limiter-holding attempt.
	name := bytes.Join([][]byte{
		[]byte(workflowID),
		[]byte(runID),
		binary.LittleEndian.AppendUint64(nil, uint64(scheduledEventID)),
		componentRef,
		binary.LittleEndian.AppendUint32(nil, uint32(stamp)),
	}, []byte{0})
	return uuid.NewSHA1(uuid.NameSpaceURL, name).String()
}

func NeedsRelease(ref *taskqueuespb.LimiterRef) bool {
	switch ref.GetLimiterType() {
	case enumsspb.LIMITER_TYPE_CONCURRENCY:
		return true
	default:
		return false
	}
}
