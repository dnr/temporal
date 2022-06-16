func (c *clientImpl) DescribeTaskQueue(
	ctx context.Context,
	request *workflowservice.DescribeTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeTaskQueueResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.DescribeTaskQueue(ctx, request, opts...)
}
