func (c *clientImpl) RecordChildExecutionCompleted(
	ctx context.Context,
	request *historyservice.RecordChildExecutionCompletedRequest,
	opts ...grpc.CallOption) (*historyservice.RecordChildExecutionCompletedResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.WorkflowExecution.WorkflowId)
	if err != nil {
		return nil, err
	}

	var response *historyservice.RecordChildExecutionCompletedResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.RecordChildExecutionCompleted(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil

}
