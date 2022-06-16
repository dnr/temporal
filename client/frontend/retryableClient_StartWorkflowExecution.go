func (c *retryableClient) StartWorkflowExecution(
	ctx context.Context,
	request *workflowservice.StartWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.StartWorkflowExecutionResponse, error) {
	var resp *workflowservice.StartWorkflowExecutionResponse
	op := func() error {
		var err error
		resp, err = c.client.StartWorkflowExecution(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
