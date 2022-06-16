func (c *retryableClient) UpdateSchedule(
	ctx context.Context,
	request *workflowservice.UpdateScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.UpdateScheduleResponse, error) {
	var resp *workflowservice.UpdateScheduleResponse
	op := func() error {
		var err error
		resp, err = c.client.UpdateSchedule(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
