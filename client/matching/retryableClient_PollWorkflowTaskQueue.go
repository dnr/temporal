func (c *retryableClient) PollWorkflowTaskQueue(
	ctx context.Context,
	request *matchingservice.PollWorkflowTaskQueueRequest,
	opts ...grpc.CallOption,
) (*matchingservice.PollWorkflowTaskQueueResponse, error) {
	var resp *matchingservice.PollWorkflowTaskQueueResponse
	op := func() error {
		var err error
		resp, err = c.client.PollWorkflowTaskQueue(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
