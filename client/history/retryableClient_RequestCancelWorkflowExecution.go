func (c *retryableClient) RequestCancelWorkflowExecution(
	ctx context.Context,
	request *historyservice.RequestCancelWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*historyservice.RequestCancelWorkflowExecutionResponse, error) {
	var resp *historyservice.RequestCancelWorkflowExecutionResponse
	op := func() error {
		var err error
		resp, err = c.client.RequestCancelWorkflowExecution(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
