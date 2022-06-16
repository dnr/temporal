func (c *retryableClient) ListClosedWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.ListClosedWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListClosedWorkflowExecutionsResponse, error) {
	var resp *workflowservice.ListClosedWorkflowExecutionsResponse
	op := func() error {
		var err error
		resp, err = c.client.ListClosedWorkflowExecutions(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
