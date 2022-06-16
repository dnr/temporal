func (c *clientImpl) ResetStickyTaskQueue(
	ctx context.Context,
	request *workflowservice.ResetStickyTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ResetStickyTaskQueueResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.ResetStickyTaskQueue(ctx, request, opts...)
}
