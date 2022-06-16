func (c *retryableClient) ResetStickyTaskQueue(
	ctx context.Context,
	request *workflowservice.ResetStickyTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ResetStickyTaskQueueResponse, error) {
	var resp *workflowservice.ResetStickyTaskQueueResponse
	op := func() error {
		var err error
		resp, err = c.client.ResetStickyTaskQueue(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
