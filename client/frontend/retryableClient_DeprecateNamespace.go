func (c *retryableClient) DeprecateNamespace(
	ctx context.Context,
	request *workflowservice.DeprecateNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DeprecateNamespaceResponse, error) {
	var resp *workflowservice.DeprecateNamespaceResponse
	op := func() error {
		var err error
		resp, err = c.client.DeprecateNamespace(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
