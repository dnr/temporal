func (c *clientImpl) VerifyFirstWorkflowTaskScheduled(
	ctx context.Context,
	request *historyservice.VerifyFirstWorkflowTaskScheduledRequest,
	opts ...grpc.CallOption,
) (*historyservice.VerifyFirstWorkflowTaskScheduledResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.WorkflowExecution.WorkflowId)
	if err != nil {
		return nil, err
	}
	var response *historyservice.VerifyFirstWorkflowTaskScheduledResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.VerifyFirstWorkflowTaskScheduled(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
