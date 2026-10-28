package concurrency

import (
	"context"
	"sync"
	"time"

	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/dynamicconfig"
)

const (
	PreReleaseModeOff   = "off"   // don't release from frontend, history's release task does it
	PreReleaseModeAsync = "async" // release from frontend but don't wait, history still creates the release task
	PreReleaseModeSync  = "sync"  // release from frontend and wait, history skips the release task on success
)

type PreReleaseOptions struct {
	Mode    string
	Timeout time.Duration
}

var defaultPreReleaseOptions = PreReleaseOptions{
	Mode:    PreReleaseModeSync,
	Timeout: 500 * time.Millisecond,
}

var PreRelease = dynamicconfig.NewGlobalTypedSetting(
	"flowcontrol.concurrency.preRelease",
	defaultPreReleaseOptions,
	`Whether frontend releases concurrency limiter slots from the task token when a worker
responds to an activity task, before history records the completion. Mode is "off", "async"
(don't wait, history still creates a release task), or "sync" (wait up to Timeout, and history
skips the release task if the release succeeded).`,
)

var FrontendClientBatcherOptions = dynamicconfig.NewGlobalTypedSetting(
	"flowcontrol.concurrency.frontendClientBatcher",
	defaultClientBatcherOptions,
	`Batcher options for concurrency limiter releases from frontend. Requires server restart.`,
)

// PreReleaser releases limiter slots from frontend when a worker reports that a task attempt is
// done. A nil PreReleaser does nothing.
type PreReleaser struct {
	client fcpb.ConcurrencyServiceClient
	opts   dynamicconfig.TypedPropertyFn[PreReleaseOptions]
}

func NewPreReleaser(
	client fcpb.ConcurrencyServiceClient,
	dc *dynamicconfig.Collection,
	timeSource clock.TimeSource,
) *PreReleaser {
	return &PreReleaser{
		client: NewBatchingClient(client, FrontendClientBatcherOptions.Get(dc)(), timeSource, nil),
		opts:   PreRelease.Get(dc),
	}
}

// Release releases the slots in refs according to the configured mode. It returns the refs
// that were definitely released, which history doesn't need to create a release task for.
// Releasing is idempotent (by slot id), so a ref from a stale token is harmless: that slot was
// already released (or will be) when its attempt ended.
func (p *PreReleaser) Release(
	ctx context.Context,
	namespaceID string,
	refs []*taskqueuespb.LimiterRef,
) []*taskqueuespb.LimiterRef {
	if p == nil || len(refs) == 0 {
		return nil
	}
	opts := p.opts()
	switch opts.Mode {
	case PreReleaseModeSync:
		return p.release(ctx, namespaceID, refs, opts.Timeout)
	case PreReleaseModeAsync:
		go p.release(context.WithoutCancel(ctx), namespaceID, refs, opts.Timeout)
		return nil
	default:
		return nil
	}
}

func (p *PreReleaser) release(
	ctx context.Context,
	namespaceID string,
	refs []*taskqueuespb.LimiterRef,
	timeout time.Duration,
) []*taskqueuespb.LimiterRef {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ok := make([]bool, len(refs))
	var wg sync.WaitGroup
	for i, ref := range refs {
		if ref.GetLimiterType() != enumsspb.LIMITER_TYPE_CONCURRENCY {
			continue
		}
		wg.Go(func() {
			_, err := p.client.Batch(ctx, &fcpb.ConcurrencyServiceBatchRequest{
				NamespaceId:  namespaceID,
				Key:          ref.GetKey(),
				RunId:        ref.GetRunId(),
				ReleaseSlots: []string{ref.GetSlotId()},
			})
			ok[i] = err == nil
		})
	}
	wg.Wait()

	var released []*taskqueuespb.LimiterRef
	for i, ref := range refs {
		if ok[i] {
			released = append(released, ref)
		}
	}
	return released
}
