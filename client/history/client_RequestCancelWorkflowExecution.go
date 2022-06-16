func (c *clientImpl) RequestCancelWorkflowExecution(
	ctx context.Context,
	request *historyservice.RequestCancelWorkflowExecutionRequest,
	opts ...grpc.CallOption) (*historyservice.RequestCancelWorkflowExecutionResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.CancelRequest.WorkflowExecution.WorkflowId)
	if err != nil {
		return nil, err
	}

	var response *historyservice.RequestCancelWorkflowExecutionResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.RequestCancelWorkflowExecution(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
