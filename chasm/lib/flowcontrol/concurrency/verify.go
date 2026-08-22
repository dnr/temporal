package concurrency

import (
	"context"
	"errors"
	"slices"

	"go.temporal.io/api/serviceerror"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	commontaskqueue "go.temporal.io/server/common/taskqueue"
)

type limiterSlots struct {
	key     string
	slotIDs []string
}

// slotsByLimiter groups the concurrency slots referenced by refs by limiter key, preserving the
// order in which the limiters appear in refs. Limiter types that don't allocate slots (or that
// don't need an explicit release) are skipped.
func slotsByLimiter(refs []*taskqueuespb.LimiterRef) []limiterSlots {
	var out []limiterSlots
	for _, ref := range refs {
		if ref.GetLimiterType() != enumsspb.LIMITER_TYPE_CONCURRENCY ||
			!commontaskqueue.NeedsRelease(ref) {
			continue
		}
		idx := slices.IndexFunc(out, func(ls limiterSlots) bool { return ls.key == ref.GetKey() })
		if idx < 0 {
			out = append(out, limiterSlots{key: ref.GetKey(), slotIDs: []string{ref.GetSlotId()}})
			continue
		}
		out[idx].slotIDs = append(out[idx].slotIDs, ref.GetSlotId())
	}
	return out
}

// ReleaseSlots releases every slot referenced by refs. It is idempotent: releasing a slot that was
// never committed, or that was already released, is a no-op. This is the "side effect" half of the
// release protocol and only succeeds on the cluster that is active for the namespace.
//
// A failure against one limiter doesn't stop the others: the caller retries the whole set, and
// re-releasing an already released slot costs nothing.
func ReleaseSlots(
	ctx context.Context,
	client fcpb.ConcurrencyServiceClient,
	namespaceID string,
	refs []*taskqueuespb.LimiterRef,
) error {
	var releaseErrors []error
	for _, ls := range slotsByLimiter(refs) {
		if _, err := client.Batch(ctx, &fcpb.ConcurrencyBatchRequest{
			NamespaceId:  namespaceID,
			Key:          ls.key,
			ReleaseSlots: ls.slotIDs,
		}); err != nil {
			releaseErrors = append(releaseErrors, err)
		}
	}
	return errors.Join(releaseErrors...)
}

// SlotsReleased reports whether every slot referenced by refs has been released according to this
// cluster's copy of the limiter state. It is the verification half of the release protocol: a
// standby cluster runs it to find out whether the release performed by the active cluster has
// replicated here yet, and therefore whether its own copy of the release task can be dropped.
//
// A limiter that doesn't exist in this cluster reports "not released": the limiter's state lives
// on a different shard than the execution that held the slot and replicates independently, so a
// missing limiter may just mean that its state hasn't arrived yet. Reporting "released" there
// would let a standby drop a release that it still has to perform if it becomes active.
func SlotsReleased(
	ctx context.Context,
	client fcpb.ConcurrencyServiceClient,
	namespaceID string,
	refs []*taskqueuespb.LimiterRef,
) (bool, error) {
	for _, ls := range slotsByLimiter(refs) {
		res, err := client.Verify(ctx, &fcpb.ConcurrencyVerifyRequest{
			NamespaceId: namespaceID,
			Key:         ls.key,
			SlotIds:     ls.slotIDs,
		})
		switch err.(type) {
		case nil:
		case *serviceerror.NotFound, *serviceerror.NamespaceNotFound:
			return false, nil
		default:
			return false, err
		}
		if len(res.GetReleased()) != len(ls.slotIDs) {
			return false, serviceerror.NewInternal("invalid concurrency verify response")
		}
		if slices.Contains(res.GetReleased(), false) {
			return false, nil
		}
	}
	return true, nil
}
