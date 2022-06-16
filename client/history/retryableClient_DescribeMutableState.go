func (c *retryableClient) DescribeMutableState(
	ctx context.Context,
	request *historyservice.DescribeMutableStateRequest,
	opts ...grpc.CallOption) (*historyservice.DescribeMutableStateResponse, error) {

	var resp *historyservice.DescribeMutableStateResponse
	op := func() error {
		var err error
		resp, err = c.client.DescribeMutableState(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
