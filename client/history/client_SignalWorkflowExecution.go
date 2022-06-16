func (c *clientImpl) SignalWorkflowExecution(
	ctx context.Context,
	request *historyservice.SignalWorkflowExecutionRequest,
	opts ...grpc.CallOption) (*historyservice.SignalWorkflowExecutionResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.SignalRequest.WorkflowExecution.WorkflowId)
	if err != nil {
		return nil, err
	}

	var response *historyservice.SignalWorkflowExecutionResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.SignalWorkflowExecution(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil

}
