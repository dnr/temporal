func (c *retryableClient) CreateSchedule(
	ctx context.Context,
	request *workflowservice.CreateScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.CreateScheduleResponse, error) {
	var resp *workflowservice.CreateScheduleResponse
	op := func() error {
		var err error
		resp, err = c.client.CreateSchedule(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
