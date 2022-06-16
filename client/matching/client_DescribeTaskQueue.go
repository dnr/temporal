func (c *clientImpl) DescribeTaskQueue(ctx context.Context, request *matchingservice.DescribeTaskQueueRequest, opts ...grpc.CallOption) (*matchingservice.DescribeTaskQueueResponse, error) {
	client, err := c.getClientForTaskqueue(request.DescRequest.TaskQueue.GetName())
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.DescribeTaskQueue(ctx, request, opts...)
}
