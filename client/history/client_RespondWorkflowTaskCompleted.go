func (c *clientImpl) RespondWorkflowTaskCompleted(
	ctx context.Context,
	request *historyservice.RespondWorkflowTaskCompletedRequest,
	opts ...grpc.CallOption) (*historyservice.RespondWorkflowTaskCompletedResponse, error) {
	taskToken, err := c.tokenSerializer.Deserialize(request.CompleteRequest.TaskToken)
	if err != nil {
		return nil, err
	}
	client, err := c.getClientForWorkflowID(request.NamespaceId, taskToken.GetWorkflowId())
	if err != nil {
		return nil, err
	}

	var response *historyservice.RespondWorkflowTaskCompletedResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.RespondWorkflowTaskCompleted(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	return response, err
}
