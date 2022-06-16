func (c *clientImpl) RespondWorkflowTaskFailed(
	ctx context.Context,
	request *historyservice.RespondWorkflowTaskFailedRequest,
	opts ...grpc.CallOption) (*historyservice.RespondWorkflowTaskFailedResponse, error) {
	taskToken, err := c.tokenSerializer.Deserialize(request.FailedRequest.TaskToken)
	if err != nil {
		return nil, err
	}
	client, err := c.getClientForWorkflowID(request.NamespaceId, taskToken.GetWorkflowId())
	if err != nil {
		return nil, err
	}

	var response *historyservice.RespondWorkflowTaskFailedResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.RespondWorkflowTaskFailed(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil

}
