func (c *retryableClient) SignalWorkflowExecution(
	ctx context.Context,
	request *workflowservice.SignalWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.SignalWorkflowExecutionResponse, error) {
	var resp *workflowservice.SignalWorkflowExecutionResponse
	op := func() error {
		var err error
		resp, err = c.client.SignalWorkflowExecution(ctx, request, opts...)
		return err
	}

	return resp, backoff.Retry(op, c.policy, c.isRetryable)
}
