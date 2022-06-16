func (c *clientImpl) QueryWorkflow(ctx context.Context, request *matchingservice.QueryWorkflowRequest, opts ...grpc.CallOption) (*matchingservice.QueryWorkflowResponse, error) {
	partition := c.loadBalancer.PickReadPartition(
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
	return client.QueryWorkflow(ctx, request, opts...)
}
