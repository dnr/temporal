func (c *retryableClient) ListWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.ListWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListWorkflowExecutionsResponse, error) {
	var resp *workflowservice.ListWorkflowExecutionsResponse
	op := func() error {
		var err error
		resp, err = c.client.ListWorkflowExecutions(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
