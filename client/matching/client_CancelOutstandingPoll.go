func (c *clientImpl) CancelOutstandingPoll(
	ctx context.Context,
	request *matchingservice.CancelOutstandingPollRequest,
	opts ...grpc.CallOption,
) (*matchingservice.CancelOutstandingPollResponse, error) {

	client, err := c.getClientForTaskqueue(request.TaskQueue.GetName())
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.CancelOutstandingPoll(ctx, request, opts...)
}
