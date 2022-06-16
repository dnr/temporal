func (c *retryableClient) RespondWorkflowTaskFailed(
	ctx context.Context,
	request *workflowservice.RespondWorkflowTaskFailedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondWorkflowTaskFailedResponse, error) {
	var resp *workflowservice.RespondWorkflowTaskFailedResponse
	op := func() error {
		var err error
		resp, err = c.client.RespondWorkflowTaskFailed(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
