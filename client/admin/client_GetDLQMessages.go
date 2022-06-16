func (c *clientImpl) GetDLQMessages(
	ctx context.Context,
	request *adminservice.GetDLQMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetDLQMessagesResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.GetDLQMessages(ctx, request, opts...)
}
