func (c *retryableClient) DescribeTaskQueue(
	ctx context.Context,
	request *matchingservice.DescribeTaskQueueRequest,
	opts ...grpc.CallOption) (*matchingservice.DescribeTaskQueueResponse, error) {

	var resp *matchingservice.DescribeTaskQueueResponse
	op := func() error {
		var err error
		resp, err = c.client.DescribeTaskQueue(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
