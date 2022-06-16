func (c *clientImpl) GenerateLastHistoryReplicationTasks(
	ctx context.Context,
	request *historyservice.GenerateLastHistoryReplicationTasksRequest,
	opts ...grpc.CallOption,
) (*historyservice.GenerateLastHistoryReplicationTasksResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.GetExecution().GetWorkflowId())
	if err != nil {
		return nil, err
	}

	var response *historyservice.GenerateLastHistoryReplicationTasksResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.GenerateLastHistoryReplicationTasks(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
