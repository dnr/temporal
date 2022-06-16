func (c *clientImpl) UpdateNamespace(
	ctx context.Context,
	request *workflowservice.UpdateNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.UpdateNamespaceResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.UpdateNamespace(ctx, request, opts...)
}
