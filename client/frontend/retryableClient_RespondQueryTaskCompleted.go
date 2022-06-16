func (c *retryableClient) RespondQueryTaskCompleted(
	ctx context.Context,
	request *workflowservice.RespondQueryTaskCompletedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondQueryTaskCompletedResponse, error) {
	var resp *workflowservice.RespondQueryTaskCompletedResponse
	op := func() error {
		var err error
		resp, err = c.client.RespondQueryTaskCompleted(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
