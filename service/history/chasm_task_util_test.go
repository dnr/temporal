package history

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	"go.temporal.io/server/chasm"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/service/history/tasks"
)

func TestBypassTaskGenerationValidation(t *testing.T) {
	registry := chasm.NewRegistry(log.NewTestLogger())
	require.NoError(t, registry.Register(&standbyVerifiableTaskTestLibrary{}))
	taskTypeID, ok := registry.TaskIDFor(&standbyVerifiableTestTask{})
	require.True(t, ok)

	require.True(t, bypassTaskGenerationValidation(&tasks.ReleaseLimiterTask{}, registry))
	require.True(t, bypassTaskGenerationValidation(&tasks.ChasmTask{
		Info: &persistencespb.ChasmTaskInfo{TypeId: taskTypeID},
	}, registry))
	require.False(t, bypassTaskGenerationValidation(&tasks.ChasmTask{
		Info: &persistencespb.ChasmTaskInfo{TypeId: taskTypeID + 1},
	}, registry))
}

// discardableTaskTestLibrary is a minimal CHASM library that registers a side-effect task whose handler has a custom
// Discard implementation, used for testing discard paths in standby task executors.
type discardableTaskTestLibrary struct {
	chasm.UnimplementedLibrary
}

func (l *discardableTaskTestLibrary) Name() string { return "DiscardableTestLib" }

func (l *discardableTaskTestLibrary) Tasks() []*chasm.RegistrableTask {
	return []*chasm.RegistrableTask{
		chasm.NewRegistrableSideEffectTask(
			"discard_task",
			&discardableTestTaskHandler{},
		),
	}
}

type discardableTestTask struct{}

type discardableTestTaskHandler struct {
	chasm.SideEffectTaskHandlerBase[*discardableTestTask]
}

func (e *discardableTestTaskHandler) Validate(_ chasm.Context, _ any, _ chasm.TaskInvocation, _ *discardableTestTask) (bool, error) {
	return true, nil
}

func (e *discardableTestTaskHandler) Execute(_ context.Context, _ chasm.ComponentRef, _ chasm.TaskAttributes, _ *discardableTestTask) error {
	return nil
}

func (e *discardableTestTaskHandler) Discard(_ context.Context, _ chasm.ComponentRef, _ chasm.TaskAttributes, _ *discardableTestTask) error {
	return nil
}

// nonDiscardableTaskTestLibrary is a minimal CHASM library that registers a side-effect task whose handler uses the
// default Discard from SideEffectTaskHandlerBase (returns ErrTaskDiscarded).
type nonDiscardableTaskTestLibrary struct {
	chasm.UnimplementedLibrary
}

func (l *nonDiscardableTaskTestLibrary) Name() string { return "NonDiscardableTestLib" }

func (l *nonDiscardableTaskTestLibrary) Tasks() []*chasm.RegistrableTask {
	return []*chasm.RegistrableTask{
		chasm.NewRegistrableSideEffectTask(
			"non_discard_task",
			&nonDiscardableTestTaskHandler{},
		),
	}
}

type nonDiscardableTestTask struct{}

type nonDiscardableTestTaskHandler struct {
	chasm.SideEffectTaskHandlerBase[*nonDiscardableTestTask]
}

func (e *nonDiscardableTestTaskHandler) Validate(_ chasm.Context, _ any, _ chasm.TaskInvocation, _ *nonDiscardableTestTask) (bool, error) {
	return true, nil
}

func (e *nonDiscardableTestTaskHandler) Execute(_ context.Context, _ chasm.ComponentRef, _ chasm.TaskAttributes, _ *nonDiscardableTestTask) error {
	return nil
}

type standbyVerifiableTaskTestLibrary struct {
	chasm.UnimplementedLibrary
}

func (l *standbyVerifiableTaskTestLibrary) Name() string { return "StandbyVerifiableTestLib" }

func (l *standbyVerifiableTaskTestLibrary) Tasks() []*chasm.RegistrableTask {
	return []*chasm.RegistrableTask{
		chasm.NewRegistrableSideEffectTask(
			"standby_verifiable_task",
			&standbyVerifiableTestTaskHandler{},
		),
	}
}

type standbyVerifiableTestTask struct{}

type standbyVerifiableTestTaskHandler struct {
	chasm.SideEffectTaskHandlerBase[*standbyVerifiableTestTask]
}

func (e *standbyVerifiableTestTaskHandler) Validate(_ chasm.Context, _ any, _ chasm.TaskInvocation, _ *standbyVerifiableTestTask) (bool, error) {
	return true, nil
}

func (e *standbyVerifiableTestTaskHandler) Execute(_ context.Context, _ chasm.ComponentRef, _ chasm.TaskAttributes, _ *standbyVerifiableTestTask) error {
	return nil
}

func (e *standbyVerifiableTestTaskHandler) ExecuteStandby(_ context.Context, _ chasm.ComponentRef, _ chasm.StandbyTaskInvocation, _ *standbyVerifiableTestTask) error {
	return nil
}
