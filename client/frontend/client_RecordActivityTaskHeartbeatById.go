func (c *clientImpl) RecordActivityTaskHeartbeatById(
	ctx context.Context,
	request *workflowservice.RecordActivityTaskHeartbeatByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RecordActivityTaskHeartbeatByIdResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.RecordActivityTaskHeartbeatById(ctx, request, opts...)
}
