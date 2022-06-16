func (c *clientImpl) PurgeDLQMessages(
	ctx context.Context,
	request *historyservice.PurgeDLQMessagesRequest,
	opts ...grpc.CallOption,
) (*historyservice.PurgeDLQMessagesResponse, error) {

	client, err := c.getClientForShardID(request.GetShardId())
	if err != nil {
		return nil, err
	}
	return client.PurgeDLQMessages(ctx, request, opts...)
}
