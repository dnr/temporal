func (c *retryableClient) GetWorkflowExecutionHistory(
	ctx context.Context,
	request *workflowservice.GetWorkflowExecutionHistoryRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetWorkflowExecutionHistoryResponse, error) {
	var resp *workflowservice.GetWorkflowExecutionHistoryResponse
	op := func() error {
		var err error
		resp, err = c.client.GetWorkflowExecutionHistory(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
