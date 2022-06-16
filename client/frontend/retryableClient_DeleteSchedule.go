func (c *retryableClient) DeleteSchedule(
	ctx context.Context,
	request *workflowservice.DeleteScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DeleteScheduleResponse, error) {
	var resp *workflowservice.DeleteScheduleResponse
	op := func() error {
		var err error
		resp, err = c.client.DeleteSchedule(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
