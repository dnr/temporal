func (c *clientImpl) PollActivityTaskQueue(
	ctx context.Context,
	request *workflowservice.PollActivityTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PollActivityTaskQueueResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createLongPollContext(ctx)
	defer cancel()
	return client.PollActivityTaskQueue(ctx, request, opts...)
}
