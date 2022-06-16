func (c *retryableClient) ListTaskQueuePartitions(
	ctx context.Context,
	request *workflowservice.ListTaskQueuePartitionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListTaskQueuePartitionsResponse, error) {
	var resp *workflowservice.ListTaskQueuePartitionsResponse
	op := func() error {
		var err error
		resp, err = c.client.ListTaskQueuePartitions(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
