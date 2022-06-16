func (c *clientImpl) RespondActivityTaskFailed(
	ctx context.Context,
	request *historyservice.RespondActivityTaskFailedRequest,
	opts ...grpc.CallOption) (*historyservice.RespondActivityTaskFailedResponse, error) {
	taskToken, err := c.tokenSerializer.Deserialize(request.FailedRequest.TaskToken)
	if err != nil {
		return nil, err
	}
	client, err := c.getClientForWorkflowID(request.NamespaceId, taskToken.GetWorkflowId())
	if err != nil {
		return nil, err
	}

	var response *historyservice.RespondActivityTaskFailedResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.RespondActivityTaskFailed(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil

}
