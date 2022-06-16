func (c *clientImpl) ListClusters(
	ctx context.Context,
	request *adminservice.ListClustersRequest,
	opts ...grpc.CallOption,
) (*adminservice.ListClustersResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.ListClusters(ctx, request, opts...)
}
