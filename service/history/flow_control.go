package history

import (
	"context"

	"go.temporal.io/api/serviceerror"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
)

func verifyLimiterReleases(
	ctx context.Context,
	client fcpb.ConcurrencyServiceClient,
	namespaceID string,
	releases []*taskqueuespb.LimiterRelease,
) (bool, error) {
	for _, release := range releases {
		limiter := release.GetLimiter()
		if len(release.GetComponentRef()) == 0 {
			return false, nil
		}
		response, err := client.VerifyRelease(ctx, &fcpb.VerifyReleaseRequest{
			NamespaceId:  namespaceID,
			Key:          limiter.GetKey(),
			Slots:        []string{limiter.GetSlotId()},
			ComponentRef: release.GetComponentRef(),
		})
		switch err.(type) {
		case nil:
			if !response.GetReleased() {
				return false, nil
			}
		case *serviceerror.NotFound, *serviceerror.WorkflowNotReady, *serviceerror.Unavailable:
			return false, nil
		default:
			return false, err
		}
	}
	return true, nil
}
