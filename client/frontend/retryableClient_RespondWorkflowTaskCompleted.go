func (c *retryableClient) RespondWorkflowTaskCompleted(
	ctx context.Context,
	request *workflowservice.RespondWorkflowTaskCompletedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondWorkflowTaskCompletedResponse, error) {
	var resp *workflowservice.RespondWorkflowTaskCompletedResponse
	op := func() error {
		var err error
		resp, err = c.client.RespondWorkflowTaskCompleted(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
