func (c *clientImpl) GetSystemInfo(
	ctx context.Context,
	request *workflowservice.GetSystemInfoRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetSystemInfoResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.GetSystemInfo(ctx, request, opts...)
}
