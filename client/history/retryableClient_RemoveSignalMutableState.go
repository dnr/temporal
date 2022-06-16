func (c *retryableClient) RemoveSignalMutableState(
	ctx context.Context,
	request *historyservice.RemoveSignalMutableStateRequest,
	opts ...grpc.CallOption) (*historyservice.RemoveSignalMutableStateResponse, error) {

	var resp *historyservice.RemoveSignalMutableStateResponse
	op := func() error {
		var err error
		resp, err = c.client.RemoveSignalMutableState(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
