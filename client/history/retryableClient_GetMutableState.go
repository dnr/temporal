func (c *retryableClient) GetMutableState(
	ctx context.Context,
	request *historyservice.GetMutableStateRequest,
	opts ...grpc.CallOption) (*historyservice.GetMutableStateResponse, error) {

	var resp *historyservice.GetMutableStateResponse
	op := func() error {
		var err error
		resp, err = c.client.GetMutableState(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
