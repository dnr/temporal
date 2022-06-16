func (c *clientImpl) AddActivityTask(
	ctx context.Context,
	request *matchingservice.AddActivityTaskRequest,
	opts ...grpc.CallOption) (*matchingservice.AddActivityTaskResponse, error) {
	partition := c.loadBalancer.PickWritePartition(
		namespace.ID(request.GetNamespaceId()),
		*request.GetTaskQueue(),
		enumspb.TASK_QUEUE_TYPE_ACTIVITY,
		request.GetForwardedSource(),
	)
	request.TaskQueue.Name = partition
	client, err := c.getClientForTaskqueue(partition)
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.AddActivityTask(ctx, request, opts...)
}
