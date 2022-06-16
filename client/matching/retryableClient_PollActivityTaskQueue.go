func (c *retryableClient) PollActivityTaskQueue(
	ctx context.Context,
	request *matchingservice.PollActivityTaskQueueRequest,
	opts ...grpc.CallOption,
) (*matchingservice.PollActivityTaskQueueResponse, error) {
	var resp *matchingservice.PollActivityTaskQueueResponse
	op := func() error {
		var err error
		resp, err = c.client.PollActivityTaskQueue(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
