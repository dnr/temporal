func (c *retryableClient) GetClusterInfo(
	ctx context.Context,
	request *workflowservice.GetClusterInfoRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetClusterInfoResponse, error) {
	var resp *workflowservice.GetClusterInfoResponse
	op := func() error {
		var err error
		resp, err = c.client.GetClusterInfo(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
