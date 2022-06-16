func (c *clientImpl) RebuildMutableState(
	ctx context.Context,
	request *historyservice.RebuildMutableStateRequest,
	opts ...grpc.CallOption,
) (*historyservice.RebuildMutableStateResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.Execution.WorkflowId)
	if err != nil {
		return nil, err
	}
	var response *historyservice.RebuildMutableStateResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.RebuildMutableState(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
