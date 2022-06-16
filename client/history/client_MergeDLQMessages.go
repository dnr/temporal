func (c *clientImpl) MergeDLQMessages(
	ctx context.Context,
	request *historyservice.MergeDLQMessagesRequest,
	opts ...grpc.CallOption,
) (*historyservice.MergeDLQMessagesResponse, error) {

	client, err := c.getClientForShardID(request.GetShardId())
	if err != nil {
		return nil, err
	}
	return client.MergeDLQMessages(ctx, request, opts...)
}
