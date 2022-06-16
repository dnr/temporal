func (c *retryableClient) GetWorkflowExecutionHistoryReverse(
	ctx context.Context,
	request *workflowservice.GetWorkflowExecutionHistoryReverseRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetWorkflowExecutionHistoryReverseResponse, error) {
	var resp *workflowservice.GetWorkflowExecutionHistoryReverseResponse
	op := func() error {
		var err error
		resp, err = c.client.GetWorkflowExecutionHistoryReverse(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
