func (c *clientImpl) ResetStickyTaskQueue(
	ctx context.Context,
	request *historyservice.ResetStickyTaskQueueRequest,
	opts ...grpc.CallOption,
) (*historyservice.ResetStickyTaskQueueResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.Execution.WorkflowId)
	if err != nil {
		return nil, err
	}
	var response *historyservice.ResetStickyTaskQueueResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.ResetStickyTaskQueue(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
