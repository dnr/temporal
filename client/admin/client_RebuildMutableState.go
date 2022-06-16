func (c *clientImpl) RebuildMutableState(
	ctx context.Context,
	request *adminservice.RebuildMutableStateRequest,
	opts ...grpc.CallOption,
) (*adminservice.RebuildMutableStateResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.RebuildMutableState(ctx, request, opts...)
}
