func (c *retryableClient) GetSystemInfo(
	ctx context.Context,
	request *workflowservice.GetSystemInfoRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetSystemInfoResponse, error) {
	var resp *workflowservice.GetSystemInfoResponse
	op := func() error {
		var err error
		resp, err = c.client.GetSystemInfo(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
