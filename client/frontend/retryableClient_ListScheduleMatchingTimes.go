func (c *retryableClient) ListScheduleMatchingTimes(
	ctx context.Context,
	request *workflowservice.ListScheduleMatchingTimesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListScheduleMatchingTimesResponse, error) {
	var resp *workflowservice.ListScheduleMatchingTimesResponse
	op := func() error {
		var err error
		resp, err = c.client.ListScheduleMatchingTimes(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
