func (c *retryableClient) PollMutableState(
	ctx context.Context,
	request *historyservice.PollMutableStateRequest,
	opts ...grpc.CallOption) (*historyservice.PollMutableStateResponse, error) {

	var resp *historyservice.PollMutableStateResponse
	op := func() error {
		var err error
		resp, err = c.client.PollMutableState(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
