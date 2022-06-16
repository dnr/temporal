func (c *clientImpl) ListTaskQueuePartitions(
	ctx context.Context,
	request *workflowservice.ListTaskQueuePartitionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListTaskQueuePartitionsResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.ListTaskQueuePartitions(ctx, request, opts...)
}
