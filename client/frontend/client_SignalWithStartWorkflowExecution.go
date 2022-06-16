func (c *clientImpl) SignalWithStartWorkflowExecution(
	ctx context.Context,
	request *workflowservice.SignalWithStartWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.SignalWithStartWorkflowExecutionResponse, error) {
	client, err := c.getRandomClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.SignalWithStartWorkflowExecution(ctx, request, opts...)
}
