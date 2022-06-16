func (c *retryableClient) PatchSchedule(
	ctx context.Context,
	request *workflowservice.PatchScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PatchScheduleResponse, error) {
	var resp *workflowservice.PatchScheduleResponse
	op := func() error {
		var err error
		resp, err = c.client.PatchSchedule(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
