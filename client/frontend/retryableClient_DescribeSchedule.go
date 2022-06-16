func (c *retryableClient) DescribeSchedule(
	ctx context.Context,
	request *workflowservice.DescribeScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeScheduleResponse, error) {
	var resp *workflowservice.DescribeScheduleResponse
	op := func() error {
		var err error
		resp, err = c.client.DescribeSchedule(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
