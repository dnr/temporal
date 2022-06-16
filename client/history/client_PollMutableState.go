func (c *clientImpl) PollMutableState(
	ctx context.Context,
	request *historyservice.PollMutableStateRequest,
	opts ...grpc.CallOption) (*historyservice.PollMutableStateResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.Execution.WorkflowId)
	if err != nil {
		return nil, err
	}

	var response *historyservice.PollMutableStateResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.PollMutableState(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
