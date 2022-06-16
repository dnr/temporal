func (c *clientImpl) ResetWorkflowExecution(
	ctx context.Context,
	request *historyservice.ResetWorkflowExecutionRequest,
	opts ...grpc.CallOption) (*historyservice.ResetWorkflowExecutionResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.ResetRequest.WorkflowExecution.WorkflowId)
	if err != nil {
		return nil, err
	}

	var response *historyservice.ResetWorkflowExecutionResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.ResetWorkflowExecution(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, err
}
