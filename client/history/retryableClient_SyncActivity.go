func (c *retryableClient) SyncActivity(
	ctx context.Context,
	request *historyservice.SyncActivityRequest,
	opts ...grpc.CallOption,
) (*historyservice.SyncActivityResponse, error) {
	var resp *historyservice.SyncActivityResponse
	op := func() error {
		var err error
		resp, err = c.client.SyncActivity(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
