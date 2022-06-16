func (c *clientImpl) RemoveRemoteCluster(
	ctx context.Context,
	request *adminservice.RemoveRemoteClusterRequest,
	opts ...grpc.CallOption,
) (*adminservice.RemoveRemoteClusterResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.RemoveRemoteCluster(ctx, request, opts...)
}
