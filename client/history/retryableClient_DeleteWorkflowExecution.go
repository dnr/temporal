func (c *retryableClient) DeleteWorkflowExecution(
	ctx context.Context,
	request *historyservice.DeleteWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*historyservice.DeleteWorkflowExecutionResponse, error) {
	var resp *historyservice.DeleteWorkflowExecutionResponse
	op := func() error {
		var err error
		resp, err = c.client.DeleteWorkflowExecution(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
