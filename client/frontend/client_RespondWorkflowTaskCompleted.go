func (c *clientImpl) RespondWorkflowTaskCompleted(
	ctx context.Context,
	request *workflowservice.RespondWorkflowTaskCompletedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondWorkflowTaskCompletedResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.RespondWorkflowTaskCompleted(ctx, request, opts...)
}
