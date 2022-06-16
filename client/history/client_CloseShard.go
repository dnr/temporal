func (c *clientImpl) CloseShard(
	ctx context.Context,
	request *historyservice.CloseShardRequest,
	opts ...grpc.CallOption,
) (*historyservice.CloseShardResponse, error) {
	client, err := c.getClientForShardID(request.GetShardId())
	if err != nil {
		return nil, err
	}

	var response *historyservice.CloseShardResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.CloseShard(ctx, request, opts...)
		return err
	}

	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
