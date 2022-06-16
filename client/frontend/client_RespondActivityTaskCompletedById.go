func (c *clientImpl) RespondActivityTaskCompletedById(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskCompletedByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskCompletedByIdResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.RespondActivityTaskCompletedById(ctx, request, opts...)
}
