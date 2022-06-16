func (c *clientImpl) DeleteWorkflowVisibilityRecord(
	ctx context.Context,
	request *historyservice.DeleteWorkflowVisibilityRecordRequest,
	opts ...grpc.CallOption,
) (*historyservice.DeleteWorkflowVisibilityRecordResponse, error) {
	client, err := c.getClientForWorkflowID(request.NamespaceId, request.GetExecution().GetWorkflowId())
	if err != nil {
		return nil, err
	}

	var response *historyservice.DeleteWorkflowVisibilityRecordResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.DeleteWorkflowVisibilityRecord(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
