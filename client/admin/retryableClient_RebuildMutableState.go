func (c *retryableClient) RebuildMutableState(
	ctx context.Context,
	request *adminservice.RebuildMutableStateRequest,
	opts ...grpc.CallOption,
) (*adminservice.RebuildMutableStateResponse, error) {
	var resp *adminservice.RebuildMutableStateResponse
	op := func() error {
		var err error
		resp, err = c.client.RebuildMutableState(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
