func (c *retryableClient) TerminateWorkflowExecution(
	ctx context.Context,
	request *workflowservice.TerminateWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.TerminateWorkflowExecutionResponse, error) {
	var resp *workflowservice.TerminateWorkflowExecutionResponse
	op := func() error {
		var err error
		resp, err = c.client.TerminateWorkflowExecution(ctx, request, opts...)
		return err
	}

	return resp, backoff.Retry(op, c.policy, c.isRetryable)
}
