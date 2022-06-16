func (c *clientImpl) PollActivityTaskQueue(
	ctx context.Context,
	request *matchingservice.PollActivityTaskQueueRequest,
	opts ...grpc.CallOption) (*matchingservice.PollActivityTaskQueueResponse, error) {
	partition := c.loadBalancer.PickReadPartition(
		namespace.ID(request.GetNamespaceId()),
		*request.PollRequest.GetTaskQueue(),
		enumspb.TASK_QUEUE_TYPE_ACTIVITY,
		request.GetForwardedSource(),
	)
	request.PollRequest.TaskQueue.Name = partition
	client, err := c.getClientForTaskqueue(request.PollRequest.TaskQueue.GetName())
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createLongPollContext(ctx)
	defer cancel()
	return client.PollActivityTaskQueue(ctx, request, opts...)
}
