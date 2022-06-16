func (c *clientImpl) VerifyChildExecutionCompletionRecorded(
	ctx context.Context,
	request *historyservice.VerifyChildExecutionCompletionRecordedRequest,
	opts ...grpc.CallOption,
) (*historyservice.VerifyChildExecutionCompletionRecordedResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.ParentExecution.WorkflowId)
	if err != nil {
		return nil, err
	}
	var response *historyservice.VerifyChildExecutionCompletionRecordedResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.VerifyChildExecutionCompletionRecorded(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
