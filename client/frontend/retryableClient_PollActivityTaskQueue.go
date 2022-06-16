func (c *retryableClient) PollActivityTaskQueue(
	ctx context.Context,
	request *workflowservice.PollActivityTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PollActivityTaskQueueResponse, error) {
	var resp *workflowservice.PollActivityTaskQueueResponse
	op := func() error {
		var err error
		resp, err = c.client.PollActivityTaskQueue(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
