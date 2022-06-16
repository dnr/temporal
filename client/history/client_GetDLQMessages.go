func (c *clientImpl) GetDLQMessages(
	ctx context.Context,
	request *historyservice.GetDLQMessagesRequest,
	opts ...grpc.CallOption,
) (*historyservice.GetDLQMessagesResponse, error) {

	client, err := c.getClientForShardID(request.GetShardId())
	if err != nil {
		return nil, err
	}
	return client.GetDLQMessages(ctx, request, opts...)
}
