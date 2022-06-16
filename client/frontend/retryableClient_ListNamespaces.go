func (c *retryableClient) ListNamespaces(
	ctx context.Context,
	request *workflowservice.ListNamespacesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListNamespacesResponse, error) {
	var resp *workflowservice.ListNamespacesResponse
	op := func() error {
		var err error
		resp, err = c.client.ListNamespaces(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
