func (c *retryableClient) RebuildMutableState(
	ctx context.Context,
	request *historyservice.RebuildMutableStateRequest,
	opts ...grpc.CallOption) (*historyservice.RebuildMutableStateResponse, error) {

	var resp *historyservice.RebuildMutableStateResponse
	op := func() error {
		var err error
		resp, err = c.client.RebuildMutableState(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
