func (c *clientImpl) StartWorkflowExecution(
	ctx context.Context,
	request *historyservice.StartWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*historyservice.StartWorkflowExecutionResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.StartRequest.WorkflowId)
	if err != nil {
		return nil, err
	}
	var response *historyservice.StartWorkflowExecutionResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.StartWorkflowExecution(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
