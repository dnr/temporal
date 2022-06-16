func (c *clientImpl) RecordWorkflowTaskStarted(
	ctx context.Context,
	request *historyservice.RecordWorkflowTaskStartedRequest,
	opts ...grpc.CallOption) (*historyservice.RecordWorkflowTaskStartedResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.WorkflowExecution.WorkflowId)
	if err != nil {
		return nil, err
	}

	var response *historyservice.RecordWorkflowTaskStartedResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.RecordWorkflowTaskStarted(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
