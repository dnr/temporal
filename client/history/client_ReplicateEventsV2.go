func (c *clientImpl) ReplicateEventsV2(
	ctx context.Context,
	request *historyservice.ReplicateEventsV2Request,
	opts ...grpc.CallOption) (*historyservice.ReplicateEventsV2Response, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.WorkflowExecution.GetWorkflowId())
	if err != nil {
		return nil, err
	}

	var response *historyservice.ReplicateEventsV2Response
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.ReplicateEventsV2(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil

}
