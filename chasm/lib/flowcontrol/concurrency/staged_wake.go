package concurrency

import "go.temporal.io/server/chasm"

type StagedWakeHandler struct {
	chasm.PureTaskHandlerBase

	handler *Handler
}

func NewStagedWakeHandler(handler *Handler) *StagedWakeHandler {
	return &StagedWakeHandler{
		handler: handler,
	}
}

type StagedWake struct {
	foo string
}

func (t *StagedWakeHandler) Validate(ctx chasm.Context, c *Component, ti chasm.TaskInvocation, w *StagedWake) (bool, error) {
	// FIXME: validate here
	return true, nil
}

func (t *StagedWakeHandler) Execute(ctx chasm.MutableContext, c *Component, ta chasm.TaskAttributes, w *StagedWake) error {
	// FIXME: execute here
	return nil
}
