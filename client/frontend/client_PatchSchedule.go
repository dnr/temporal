func (c *clientImpl) PatchSchedule(
	ctx context.Context,
	request *workflowservice.PatchScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PatchScheduleResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()

	return client.PatchSchedule(ctx, request, opts...)
}
