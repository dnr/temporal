func (c *retryableClient) ListTaskQueuePartitions(
	ctx context.Context,
	request *matchingservice.ListTaskQueuePartitionsRequest,
	opts ...grpc.CallOption) (*matchingservice.ListTaskQueuePartitionsResponse, error) {

	var resp *matchingservice.ListTaskQueuePartitionsResponse
	op := func() error {
		var err error
		resp, err = c.client.ListTaskQueuePartitions(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
