package fc

import (
	"context"
	"errors"
	"time"

	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/headers"
	"go.temporal.io/server/common/namespace"
	serviceerrors "go.temporal.io/server/common/serviceerror"
	"go.temporal.io/server/common/util"
)

const concurrencyBlockedRetention = time.Minute

var ErrConcurrencyBlocked = serviceerror.NewFailedPrecondition("blocked by concurrency limit")

type concurrencyState struct {
	limiterStateBase

	generation int64
	lastUsed   int32
	tokens     int32
	// tokensSeq is incremented whenever tokens is overwritten from a server hint. When we take
	// a token in check, we record the sequence number, so that in cancelCheck, we can avoid
	// returning a token if tokens has been superseded since check.
	tokensSeq int64
	// outstanding is the number of check tokens taken on this node whose Reserve hasn't
	// completed yet. completed is the cumulative number of those whose Reserve completed.
	// outstanding+completed is the number of reserves we've claimed, comparable to the
	// server's applied reserve seq for this client: the difference is the number of our
	// claimed slots that aren't reflected in server state yet.
	outstanding int32
	completed   int64
	// invariant: {len(waiters) > 0} == {Wait goroutine is running} == {goroCancel != nil}
	// (for now, until we add eviction)
	goroCancel         context.CancelFunc
	waiterWakePriority wakePriority // wake priority in use by current call
	// The slots hint is the latest limiter state reported by the concurrency service, either
	// directly or passed on by another partition. Unlike tokens, it doesn't include Wait
	// responses, which are wake signals for this node only, or tokens taken locally since. This
	// is what we pass on to other partitions.
	// TODO(fc): generation is not enough to really exclude stale hints, it only updates when
	// slots goes to zero. we could use a server-side "hintSeq" that has more granularity than
	// generation.
	slotsHintGeneration int64
	slotsHint           int32
	hasSlotsHint        bool
}

func (r *Readiness) getConcurrencyLimiter(nsID namespace.ID, key string) *concurrencyState {
	return r.getShard(nsID).getConcurrencyLimiter(nsID, key)
}

func (rs *readinessShard) getConcurrencyLimiter(nsID namespace.ID, key string) *concurrencyState {
	rs.lock.Lock()
	defer rs.lock.Unlock()

	mapkey := nsID.String() + key
	if cs, ok := rs.concurrencyLimiters[mapkey]; ok {
		rs.pinLocked(cs)
		cs.lastUsed = rs.r.elapsedSeconds(rs.r.timeSource.Now())
		return cs
	}
	cs := &concurrencyState{
		limiterStateBase: limiterStateBase{rs: rs, pins: 1, mapKey: mapkey, namespaceLen: uint16(len(nsID)), concurrency: true},
		// optimistically assume unknown limiter has one slot, so we call Reserve first.
		// we'll get the real number of slots as a hint.
		tokens:   1,
		lastUsed: rs.r.elapsedSeconds(rs.r.timeSource.Now()),
	}
	rs.concurrencyLimiters[mapkey] = cs
	rs.created++
	return cs
}

// check takes a check token if available. It returns the sequence of the token taken, to be
// passed to cancelCheck.
func (cs *concurrencyState) check(cb ReadinessCallback, pri wakePriority) (tokensSeq int64, err error) {
	demand := int32(1)
	if cb != nil {
		demand = wakeDemand(cb)
	}
	err = cs.rs.update(cs, notifyAsync, func() error {
		if cs.tokens == 0 {
			cs.rs.addEdgeLocked(cs, cb, pri, demand)
			return ErrConcurrencyBlocked
		}

		// remove in case it was present before
		cs.rs.removeEdgeLocked(cs, cb)
		// take one check token
		cs.tokens--
		cs.outstanding++
		tokensSeq = cs.tokensSeq
		return nil
	})
	return
}

func (cs *concurrencyState) cancelCheck(tokensSeq int64, outstanding bool) {
	cs.rs.update(cs, notifyAsync, func() error {
		if outstanding {
			cs.outstanding = max(0, cs.outstanding-1)
		}
		if tokensSeq == cs.tokensSeq {
			// this could theoretically go over the limit but it doesn't matter here
			cs.tokens++
		}
		return nil
	})
}

func (cs *concurrencyState) reserve(
	ctx context.Context,
	slotID string,
	configUpdate *taskqueuepb.ConcurrencyLimit,
	configUpdateVersion int64,
	counted int32,
) error {
	// Note that the BatchingClient reports limiter state from the response to
	// reportConcurrencySlotsHint.
	// TODO(fc): consider doing this from history to make invalid tasks cheaper to invalidate
	res, err := cs.rs.r.concurrencyServiceClient.Batch(ctx, &fcpb.ConcurrencyServiceBatchRequest{
		NamespaceId:         cs.namespaceID().String(),
		Key:                 cs.limiterKey(),
		ReserveSlots:        []string{slotID},
		CountedReserves:     counted,
		ConfigUpdate:        configUpdate,
		ConfigUpdateVersion: configUpdateVersion,
	})
	if err != nil {
		return err
	}
	if !res.ReserveSuccess[0] {
		return serviceerrors.NewFlowControlBlocked()
	}
	return nil
}

func (cs *concurrencyState) commit(ctx context.Context, slotID string) error {
	res, err := cs.rs.r.concurrencyServiceClient.Batch(ctx, &fcpb.ConcurrencyServiceBatchRequest{
		NamespaceId: cs.namespaceID().String(),
		Key:         cs.limiterKey(),
		CommitSlots: []string{slotID},
	})
	if err != nil {
		return err
	}
	if !res.CommitSuccess[0] {
		return errCommitFailure
	}
	return nil
}

func (cs *concurrencyState) cancelReserve(ctx context.Context, slotID string) {
	// The poll context may already be canceled when rollback runs.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cs.rs.r.config.cancelReservationTimeout())
	cs.rs.lock.Lock()
	cs.pins++
	cs.rs.lock.Unlock()
	go func() {
		defer cs.unpin()
		defer cancel()
		// best-effort, and the BatchingClient reports limiter state from the response
		_, _ = cs.rs.r.concurrencyServiceClient.Batch(ctx, &fcpb.ConcurrencyServiceBatchRequest{
			NamespaceId:            cs.namespaceID().String(),
			Key:                    cs.limiterKey(),
			CancelReservationSlots: []string{slotID},
		})
	}()
}

// reserveDone is called when a Reserve for a checked token completes (successfully or not).
// Its effect on server state, if any, has already been reported through the hint handler.
func (cs *concurrencyState) reserveDone() {
	cs.rs.lock.Lock()
	defer cs.rs.lock.Unlock()
	cs.outstanding = max(0, cs.outstanding-1)
	cs.completed++
}

// dropOutstanding is called when a checked token is dropped without a Reserve.
func (cs *concurrencyState) dropOutstanding() {
	cs.rs.lock.Lock()
	defer cs.rs.lock.Unlock()
	cs.outstanding = max(0, cs.outstanding-1)
}

// claimedLocked returns the number of reserves claimed by this node (see outstanding).
func (cs *concurrencyState) claimedLocked() int64 {
	return int64(cs.outstanding) + cs.completed
}

// reportConcurrencySlotsHint is the BatchingClient's HintHandler. Callers of Batch hold pins on
// the limiter, so we don't need to create it if it's missing.
func (r *Readiness) reportConcurrencySlotsHint(namespaceID, key string, gen int64, slots int32, appliedSeq int64) {
	rs := r.getShard(namespace.ID(namespaceID))
	rs.lock.Lock()
	cs := rs.concurrencyLimiters[namespaceID+key]
	rs.lock.Unlock()
	if cs != nil {
		cs.reportSlotsHintWithApplied(gen, slots, appliedSeq, notifyAsync)
	}
}

// reportSlotsHint updates cached state from available slots reported by the concurrency
// service, either directly or passed on by another partition.
func (cs *concurrencyState) reportSlotsHint(gen int64, slots int32, mode notifyMode) {
	cs.reportSlotsHintWithApplied(gen, slots, -1, mode)
}

// reportSlotsHintWithApplied is like reportSlotsHint, where appliedSeq is the server's applied
// reserve seq for this node, or -1 if the hint came from another node.
func (cs *concurrencyState) reportSlotsHintWithApplied(gen int64, slots int32, appliedSeq int64, mode notifyMode) {
	_ = cs.rs.update(cs, mode, func() error {
		// Our claimed reserves that the server hasn't applied aren't reflected in slots.
		unreflected := int64(cs.outstanding)
		if appliedSeq >= 0 {
			unreflected = max(0, cs.claimedLocked()-appliedSeq)
		}
		local := int32(max(0, int64(slots)-unreflected))
		if cs.updateTokensLocked(gen, local) {
			cs.slotsHintGeneration = gen
			cs.slotsHint = slots
			cs.hasSlotsHint = true
		}
		return nil
	})
}

// reportWaitResponse updates cached state from a Wait response. Wake tokens are a signal to
// this node only, so they don't update the slots hint that we pass on to other partitions.
func (cs *concurrencyState) reportWaitResponse(gen int64, wakeTokens int32, sentClaimed int64, mode notifyMode) {
	_ = cs.rs.update(cs, mode, func() error {
		// The server already subtracted our unreflected claims as of when we sent the request.
		// Subtract any claims we've made since then.
		cs.updateTokensLocked(gen, int32(max(0, int64(wakeTokens)-(cs.claimedLocked()-sentClaimed))))
		return nil
	})
}

// updateTokensLocked returns false if gen is older than our current state.
func (cs *concurrencyState) updateTokensLocked(gen int64, tokens int32) bool {
	if gen < cs.generation {
		return false
	}
	cs.generation = gen
	cs.tokens = tokens
	cs.tokensSeq++
	return true
}

// getSlotsHint returns the slots hint to pass on to other partitions, or nil if we don't have
// one.
func (cs *concurrencyState) getSlotsHint() *taskqueuespb.LimiterHint {
	cs.rs.lock.Lock()
	defer cs.rs.lock.Unlock()
	if !cs.hasSlotsHint {
		return nil
	}
	return &taskqueuespb.LimiterHint{
		LimiterType:        enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:                cs.limiterKey(),
		Generation:         cs.slotsHintGeneration,
		AvailableSlotsHint: cs.slotsHint,
	}
}

func (cs *concurrencyState) syncLocked(rsu *readinessSyncUpdate) {
	// wake as many as we can. note that we do not take the tokens here, the waiter will do
	// that after it wakes up and calls check.
	rsu.wake(int(cs.tokens))

	haveWaiters := len(rsu.waiters) > 0

	// if we have an outstanding call but wake priority decreased, cancel it and start a new one
	if cs.goroCancel != nil && haveWaiters && betterWakePriority(rsu.waiters[0].pri, cs.waiterWakePriority) {
		cs.goroCancel()
		cs.goroCancel = nil
	}

	if (cs.goroCancel != nil) == haveWaiters {
		return
	}

	if !haveWaiters {
		cs.goroCancel()
		cs.goroCancel = nil
		return
	}

	ctx := headers.SetCallerInfo(context.Background(), headers.NewCallerInfo(
		cs.namespaceID().String(), // TODO(fc): use namespace name instead of id
		headers.CallerTypeBackgroundHigh,
		"",
	))
	ctx, cs.goroCancel = context.WithCancel(ctx)
	cs.waiterWakePriority = rsu.waiters[0].pri // set here so we have a not-stale value before callWait runs
	// Wait result will be reported back through reportWaitResponse
	cs.pins++
	go cs.callWait(ctx)

	return
}

func (cs *concurrencyState) makeWaitRequest() *fcpb.ConcurrencyServiceWaitRequest {
	cs.rs.lock.Lock()
	defer cs.rs.lock.Unlock()

	waiters := cs.rs.fwd[cs]
	if len(waiters) == 0 {
		return nil
	}

	cs.waiterWakePriority = waiters[0].pri
	demand := int32(0)
	for _, w := range waiters {
		demand += w.demand
	}
	return &fcpb.ConcurrencyServiceWaitRequest{
		NamespaceId:         cs.namespaceID().String(),
		Key:                 cs.limiterKey(),
		Generation:          cs.generation,
		WakePriority:        int64(cs.waiterWakePriority),
		RequestedWakeTokens: demand,
		ClientId:            cs.rs.r.clientID,
		PendingReserveSeq:   cs.claimedLocked(),
	}
}

func (cs *concurrencyState) callWait(ctx context.Context) {
	defer cs.unpin()

	policy := backoff.NewExponentialRetryPolicy(time.Second).
		WithExpirationInterval(backoff.NoInterval).
		WithMaximumInterval(time.Minute)
	retrier := backoff.NewRetrier(policy, clock.NewRealTimeSource())

	for ctx.Err() == nil {
		req := cs.makeWaitRequest()
		if req == nil {
			// maybe race with canceling context
			util.InterruptibleSleep(ctx, retrier.NextBackOff(nil))
			continue
		}
		res, err := cs.rs.r.concurrencyServiceClient.Wait(ctx, req)
		if _, ok := errors.AsType[*serviceerror.NotFound](err); ok {
			// If the limiter doesn't exist yet, we'll get NotFound. Treat that as if we got
			// one token, so we'll allow a task through Check to Reserve, which will create it.
			res, err = &fcpb.ConcurrencyServiceWaitResponse{
				Generation: req.Generation,
				WakeTokens: 1,
			}, nil
		}
		if err != nil {
			util.InterruptibleSleep(ctx, retrier.NextBackOff(err))
			continue
		}
		retrier.Reset()

		cs.reportWaitResponse(res.Generation, res.WakeTokens, req.PendingReserveSeq, notifySync)
		// note: If we have satisfied all our waiters, then ctx
		// will be canceled before we continue this loop.
	}
}

func (cs *concurrencyState) droppableLocked(params gcParameters) bool {
	// Subtract in int32 so recent ages remain valid across the seconds counter wrapping.
	age := cs.rs.r.elapsedSeconds(params.now) - cs.lastUsed
	return cs.tokens > 0 || time.Duration(age)*time.Second >= params.concurrencyRetention
}

func (cs *concurrencyState) unpin() {
	cs.rs.unpin(cs)
}
