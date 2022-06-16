func (c *retryableClient) GetShard(
	ctx context.Context,
	request *historyservice.GetShardRequest,
	opts ...grpc.CallOption) (*historyservice.GetShardResponse, error) {

	var resp *historyservice.GetShardResponse
	op := func() error {
		var err error
		resp, err = c.client.GetShard(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
