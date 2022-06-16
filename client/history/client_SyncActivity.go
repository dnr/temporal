func (c *clientImpl) SyncActivity(
	ctx context.Context,
	request *historyservice.SyncActivityRequest,
	opts ...grpc.CallOption) (*historyservice.SyncActivityResponse, error) {

	client, err := c.getClientForWorkflowID(request.NamespaceId, request.GetWorkflowId())
	if err != nil {
		return nil, err
	}

	var response *historyservice.SyncActivityResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.SyncActivity(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil

}
