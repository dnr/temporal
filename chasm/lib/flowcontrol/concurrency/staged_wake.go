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

type stagedWake struct{}

func (t *StagedWakeHandler) Validate(ctx chasm.Context, c *Component, _ chasm.TaskInvocation, _ *stagedWake) (bool, error) {
	// If we have no slots left, there's nothing to do. And if WakeAll is already set, there's nothing to do.
	ok := c.availableSlots() > 0 && !c.WakeAll
	return ok, nil
}

func (t *StagedWakeHandler) Execute(cctx chasm.MutableContext, c *Component, _ chasm.TaskAttributes, _ *stagedWake) error {
	getWakeTime := func(wantTokens int32) (int64, bool) {
		key := batchKey{
			namespaceID: cctx.ExecutionKey().NamespaceID,
			key:         cctx.ExecutionKey().BusinessID,
		}
		return t.handler.getWakeTime(key, wantTokens)
	}
	// double number woken at each stage
	c.WakeStage++
	doWake(cctx, c, getWakeTime)
	return nil
}

func doWake(cctx chasm.MutableContext, c *Component, getWakeTime func(int32) (int64, bool)) {
	wantTokens := c.availableSlots() << c.WakeStage
	if wantTokens <= 0 {
		return // no slots available, return without modifying wake state
	}

	c.WakeUpTo, c.WakeAll = getWakeTime(wantTokens)

	if !c.WakeAll { // not all woken yet, add task for more
		cctx.AddTask(
			c,
			chasm.TaskAttributes{ScheduledTime: cctx.Now(c).Add(stagedWakeInterval)},
			&stagedWake{},
		)
	}
}
