func (c *retryableClient) QueryWorkflow(
	ctx context.Context,
	request *workflowservice.QueryWorkflowRequest,
	opts ...grpc.CallOption,
) (*workflowservice.QueryWorkflowResponse, error) {
	var resp *workflowservice.QueryWorkflowResponse
	op := func() error {
		var err error
		resp, err = c.client.QueryWorkflow(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
