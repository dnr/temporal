func (c *retryableClient) ListSchedules(
	ctx context.Context,
	request *workflowservice.ListSchedulesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListSchedulesResponse, error) {
	var resp *workflowservice.ListSchedulesResponse
	op := func() error {
		var err error
		resp, err = c.client.ListSchedules(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
