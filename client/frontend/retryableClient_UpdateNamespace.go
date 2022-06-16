func (c *retryableClient) UpdateNamespace(
	ctx context.Context,
	request *workflowservice.UpdateNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.UpdateNamespaceResponse, error) {
	var resp *workflowservice.UpdateNamespaceResponse
	op := func() error {
		var err error
		resp, err = c.client.UpdateNamespace(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
