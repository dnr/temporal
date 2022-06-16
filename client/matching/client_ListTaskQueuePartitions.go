func (c *clientImpl) ListTaskQueuePartitions(
	ctx context.Context,
	request *matchingservice.ListTaskQueuePartitionsRequest,
	opts ...grpc.CallOption,
) (*matchingservice.ListTaskQueuePartitionsResponse, error) {

	client, err := c.getClientForTaskqueue(request.TaskQueue.GetName())
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.ListTaskQueuePartitions(ctx, request, opts...)
}
