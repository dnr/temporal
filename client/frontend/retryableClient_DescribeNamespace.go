func (c *retryableClient) DescribeNamespace(
	ctx context.Context,
	request *workflowservice.DescribeNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeNamespaceResponse, error) {
	var resp *workflowservice.DescribeNamespaceResponse
	op := func() error {
		var err error
		resp, err = c.client.DescribeNamespace(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
