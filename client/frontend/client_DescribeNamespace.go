func (c *clientImpl) DescribeNamespace(
	ctx context.Context,
	request *workflowservice.DescribeNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeNamespaceResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.DescribeNamespace(ctx, request, opts...)
}
