func (c *clientImpl) GetReplicationMessages(
	ctx context.Context,
	request *adminservice.GetReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetReplicationMessagesResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContextWithLargeTimeout(ctx)
	defer cancel()
	return client.GetReplicationMessages(ctx, request, opts...)
}
