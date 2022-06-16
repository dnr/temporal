func (c *clientImpl) ScheduleWorkflowTask(
	ctx context.Context,
	request *historyservice.ScheduleWorkflowTaskRequest,
	opts ...grpc.CallOption) (*historyservice.ScheduleWorkflowTaskResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.WorkflowExecution.WorkflowId)
	if err != nil {
		return nil, err
	}

	var response *historyservice.ScheduleWorkflowTaskResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.ScheduleWorkflowTask(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil

}
