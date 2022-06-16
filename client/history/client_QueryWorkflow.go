func (c *clientImpl) QueryWorkflow(
	ctx context.Context,
	request *historyservice.QueryWorkflowRequest,
	opts ...grpc.CallOption,
) (*historyservice.QueryWorkflowResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.GetRequest().GetExecution().GetWorkflowId())
	if err != nil {
		return nil, err
	}

	var response *historyservice.QueryWorkflowResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.QueryWorkflow(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
