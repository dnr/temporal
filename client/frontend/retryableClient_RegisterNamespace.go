func (c *retryableClient) RegisterNamespace(
	ctx context.Context,
	request *workflowservice.RegisterNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RegisterNamespaceResponse, error) {
	var resp *workflowservice.RegisterNamespaceResponse
	op := func() error {
		var err error
		resp, err = c.client.RegisterNamespace(ctx, request, opts...)
		return err
	}

	return resp, backoff.Retry(op, c.policy, c.isRetryable)
}
