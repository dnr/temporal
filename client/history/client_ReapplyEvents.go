func (c *clientImpl) ReapplyEvents(
	ctx context.Context,
	request *historyservice.ReapplyEventsRequest,
	opts ...grpc.CallOption,
) (*historyservice.ReapplyEventsResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId,
		request.GetRequest().GetWorkflowExecution().GetWorkflowId())
	if err != nil {
		return nil, err
	}

	var response *historyservice.ReapplyEventsResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.ReapplyEvents(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil

}
