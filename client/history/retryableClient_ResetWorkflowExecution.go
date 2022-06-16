func (c *retryableClient) ResetWorkflowExecution(
	ctx context.Context,
	request *historyservice.ResetWorkflowExecutionRequest,
	opts ...grpc.CallOption) (*historyservice.ResetWorkflowExecutionResponse, error) {

	var resp *historyservice.ResetWorkflowExecutionResponse
	op := func() error {
		var err error
		resp, err = c.client.ResetWorkflowExecution(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
