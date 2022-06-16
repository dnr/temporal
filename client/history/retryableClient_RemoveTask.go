func (c *retryableClient) RemoveTask(
	ctx context.Context,
	request *historyservice.RemoveTaskRequest,
	opts ...grpc.CallOption) (*historyservice.RemoveTaskResponse, error) {

	var resp *historyservice.RemoveTaskResponse
	op := func() error {
		var err error
		resp, err = c.client.RemoveTask(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
