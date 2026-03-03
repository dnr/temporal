package serviceerror

import (
	errordetailsspb "go.temporal.io/server/api/errordetails/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type (
	// StalePartitionCounts represents stale partition counts error.
	StalePartitionCounts struct {
		Message string
		st      *status.Status
	}
)

// NewStalePartitionCounts returns new StalePartitionCounts error.
func NewStalePartitionCounts() error {
	return &StalePartitionCounts{Message: "stale partition counts"}
}

// Error returns string message.
func (e *StalePartitionCounts) Error() string {
	return e.Message
}

func (e *StalePartitionCounts) Status() *status.Status {
	if e.st != nil {
		return e.st
	}

	st := status.New(codes.Aborted, e.Message)
	st, _ = st.WithDetails(
		&errordetailsspb.StalePartitionCountsFailure{},
	)
	return st
}

func newStalePartitionCounts(st *status.Status) error {
	return &StalePartitionCounts{
		Message: st.Message(),
		st:      st,
	}
}
