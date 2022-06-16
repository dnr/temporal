func (c *clientImpl) RemoveTask(
	ctx context.Context,
	request *historyservice.RemoveTaskRequest,
	opts ...grpc.CallOption,
) (*historyservice.RemoveTaskResponse, error) {
	client, err := c.getClientForShardID(request.ShardId)
	if err != nil {
		return nil, err
	}
	var response *historyservice.RemoveTaskResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.RemoveTask(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
