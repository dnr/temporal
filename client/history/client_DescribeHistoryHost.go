func (c *clientImpl) DescribeHistoryHost(
	ctx context.Context,
	request *historyservice.DescribeHistoryHostRequest,
	opts ...grpc.CallOption) (*historyservice.DescribeHistoryHostResponse, error) {

	var err error
	var client historyservice.HistoryServiceClient

	if request.GetShardId() != 0 {
		client, err = c.getClientForShardID(request.GetShardId())
	} else if request.GetWorkflowExecution() != nil {
		client, err = c.getClientForWorkflowID(request.GetNamespaceId(), request.GetWorkflowExecution().GetWorkflowId())
	} else {
		ret, err := c.clients.GetClientForClientKey(request.GetHostAddress())
		if err != nil {
			return nil, err
		}
		client = ret.(historyservice.HistoryServiceClient)
	}
	if err != nil {
		return nil, err
	}

	var response *historyservice.DescribeHistoryHostResponse
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.DescribeHistoryHost(ctx, request, opts...)
		return err
	}
	err = c.executeWithRedirect(ctx, client, op)
	if err != nil {
		return nil, err
	}
	return response, nil
}
