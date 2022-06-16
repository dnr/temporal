func (c *clientImpl) RefreshWorkflowTasks(
	ctx context.Context,
	request *historyservice.RefreshWorkflowTasksRequest,
	opts ...grpc.CallOption,
) (*historyservice.RefreshWorkflowTasksResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.GetRequest().GetExecution().GetWorkflowId())
	if err != nil {
		return nil, err
	}

	var response *historyservice.RefreshWorkflowTasksResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.RefreshWorkflowTasks(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
