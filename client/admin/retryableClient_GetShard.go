func (c *retryableClient) GetShard(
	ctx context.Context,
	request *adminservice.GetShardRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetShardResponse, error) {
	var resp *adminservice.GetShardResponse
	op := func() error {
		var err error
		resp, err = c.client.GetShard(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
