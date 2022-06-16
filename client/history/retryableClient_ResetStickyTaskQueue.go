func (c *retryableClient) ResetStickyTaskQueue(
	ctx context.Context,
	request *historyservice.ResetStickyTaskQueueRequest,
	opts ...grpc.CallOption,
) (*historyservice.ResetStickyTaskQueueResponse, error) {
	var resp *historyservice.ResetStickyTaskQueueResponse
	op := func() error {
		var err error
		resp, err = c.client.ResetStickyTaskQueue(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
