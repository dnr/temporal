func (c *clientImpl) DeleteSchedule(
	ctx context.Context,
	request *workflowservice.DeleteScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DeleteScheduleResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.DeleteSchedule(ctx, request, opts...)
}
