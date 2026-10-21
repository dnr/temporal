package matching

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskFinishResult(t *testing.T) {
	t.Parallel()
	startErr := errors.New("start failed")
	forwardErr := errors.New("forward failed")
	for _, tc := range []struct {
		name   string
		finish func(*internalTask)
		want   taskResponse
	}{
		{"success", func(task *internalTask) { task.finish(nil) }, taskResponse{}},
		{"start error", func(task *internalTask) { task.finish(startErr) }, taskResponse{startErr: startErr}},
		{"drop", func(task *internalTask) { task.finish(dropReasonInvalid) }, taskResponse{dropReason: dropReasonInvalid}},
		{"forward success", func(task *internalTask) { task.finishForward("response", nil) }, taskResponse{forwarded: true, forwardRes: "response"}},
		{"forward error", func(task *internalTask) { task.finishForward(nil, forwardErr) }, taskResponse{forwarded: true, forwardErr: forwardErr}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			task := &internalTask{responseC: make(chan taskResponse, 1)}
			tc.finish(task)
			got, ok := task.getResponse()
			require.True(t, ok)
			require.Equal(t, tc.want, got)
		})
	}
}
