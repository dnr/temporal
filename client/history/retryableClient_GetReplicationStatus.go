func (c *retryableClient) GetReplicationStatus(
	ctx context.Context,
	request *historyservice.GetReplicationStatusRequest,
	opts ...grpc.CallOption,
) (*historyservice.GetReplicationStatusResponse, error) {

	var resp *historyservice.GetReplicationStatusResponse
	op := func() error {
		var err error
		resp, err = c.client.GetReplicationStatus(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
