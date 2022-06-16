func (c *clientImpl) GetDLQReplicationMessages(
	ctx context.Context,
	request *historyservice.GetDLQReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (*historyservice.GetDLQReplicationMessagesResponse, error) {
	// All workflow IDs are in the same shard per request
	client, err := c.getClientForWorkflowID(request.TaskInfos[0].NamespaceId, request.TaskInfos[0].WorkflowId)
	if err != nil {
		return nil, err
	}
	var response *historyservice.GetDLQReplicationMessagesResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.GetDLQReplicationMessages(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
