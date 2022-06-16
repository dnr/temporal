func (c *clientImpl) PollWorkflowTaskQueue(
	ctx context.Context,
	request *workflowservice.PollWorkflowTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PollWorkflowTaskQueueResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createLongPollContext(ctx)
	defer cancel()
	return client.PollWorkflowTaskQueue(ctx, request, opts...)
}
