func (c *retryableClient) SignalWorkflowExecution(
	ctx context.Context,
	request *historyservice.SignalWorkflowExecutionRequest,
	opts ...grpc.CallOption) (*historyservice.SignalWorkflowExecutionResponse, error) {

	var resp *historyservice.SignalWorkflowExecutionResponse
	op := func() error {
		var err error
		resp, err = c.client.SignalWorkflowExecution(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
