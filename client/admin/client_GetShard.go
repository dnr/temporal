func (c *clientImpl) GetShard(
	ctx context.Context,
	request *adminservice.GetShardRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetShardResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.GetShard(ctx, request, opts...)
}
