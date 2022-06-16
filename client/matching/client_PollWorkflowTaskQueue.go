func (c *clientImpl) PollWorkflowTaskQueue(
	ctx context.Context,
	request *matchingservice.PollWorkflowTaskQueueRequest,
	opts ...grpc.CallOption) (*matchingservice.PollWorkflowTaskQueueResponse, error) {
	partition := c.loadBalancer.PickReadPartition(
		namespace.ID(request.GetNamespaceId()),
		*request.PollRequest.GetTaskQueue(),
		enumspb.TASK_QUEUE_TYPE_WORKFLOW,
		request.GetForwardedSource(),
	)
	request.PollRequest.TaskQueue.Name = partition
	client, err := c.getClientForTaskqueue(request.PollRequest.TaskQueue.GetName())
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createLongPollContext(ctx)
	defer cancel()
	return client.PollWorkflowTaskQueue(ctx, request, opts...)
}
