func (c *retryableClient) CloseShard(
	ctx context.Context,
	request *historyservice.CloseShardRequest,
	opts ...grpc.CallOption) (*historyservice.CloseShardResponse, error) {

	var resp *historyservice.CloseShardResponse
	op := func() error {
		var err error
		resp, err = c.client.CloseShard(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
