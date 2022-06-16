func (c *retryableClient) DescribeTaskQueue(
	ctx context.Context,
	request *workflowservice.DescribeTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeTaskQueueResponse, error) {
	var resp *workflowservice.DescribeTaskQueueResponse
	op := func() error {
		var err error
		resp, err = c.client.DescribeTaskQueue(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
