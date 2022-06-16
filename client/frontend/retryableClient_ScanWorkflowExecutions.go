func (c *retryableClient) ScanWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.ScanWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ScanWorkflowExecutionsResponse, error) {
	var resp *workflowservice.ScanWorkflowExecutionsResponse
	op := func() error {
		var err error
		resp, err = c.client.ScanWorkflowExecutions(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
