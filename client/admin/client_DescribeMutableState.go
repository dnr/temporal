func (c *clientImpl) DescribeMutableState(
	ctx context.Context,
	request *adminservice.DescribeMutableStateRequest,
	opts ...grpc.CallOption,
) (*adminservice.DescribeMutableStateResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.DescribeMutableState(ctx, request, opts...)
}
