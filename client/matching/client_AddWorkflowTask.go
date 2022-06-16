func (c *clientImpl) AddWorkflowTask(
	ctx context.Context,
	request *matchingservice.AddWorkflowTaskRequest,
	opts ...grpc.CallOption) (*matchingservice.AddWorkflowTaskResponse, error) {
	partition := c.loadBalancer.PickWritePartition(
		namespace.ID(request.GetNamespaceId()),
		*request.GetTaskQueue(),
		enumspb.TASK_QUEUE_TYPE_WORKFLOW,
		request.GetForwardedSource(),
	)
	request.TaskQueue.Name = partition
	client, err := c.getClientForTaskqueue(request.TaskQueue.GetName())
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.AddWorkflowTask(ctx, request, opts...)
}
