func (c *clientImpl) GetDLQReplicationMessages(
	ctx context.Context,
	request *historyservice.GetDLQReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (*historyservice.GetDLQReplicationMessagesResponse, error) {
	// All workflow IDs are in the same shard per request
	namespaceID := request.GetTaskInfos()[0].GetNamespaceId()
	workflowID := request.GetTaskInfos()[0].GetWorkflowId()
	client, err := c.getClientForWorkflowID(namespaceID, workflowID)
	if err != nil {
		return nil, err
	}

	return client.GetDLQReplicationMessages(
		ctx,
		request,
		opts...,
	)
}
