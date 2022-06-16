func (c *clientImpl) SyncShardStatus(
	ctx context.Context,
	request *historyservice.SyncShardStatusRequest,
	opts ...grpc.CallOption,
) (*historyservice.SyncShardStatusResponse, error) {
	client, err := c.getClientForShardID(request.ShardId)
	if err != nil {
		return nil, err
	}
	var response *historyservice.SyncShardStatusResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.SyncShardStatus(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
