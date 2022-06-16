func (c *retryableClient) SyncShardStatus(
	ctx context.Context,
	request *historyservice.SyncShardStatusRequest,
	opts ...grpc.CallOption,
) (*historyservice.SyncShardStatusResponse, error) {
	var resp *historyservice.SyncShardStatusResponse
	op := func() error {
		var err error
		resp, err = c.client.SyncShardStatus(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
