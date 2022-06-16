func (c *retryableClient) RespondActivityTaskCompleted(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskCompletedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskCompletedResponse, error) {
	var resp *workflowservice.RespondActivityTaskCompletedResponse
	op := func() error {
		var err error
		resp, err = c.client.RespondActivityTaskCompleted(ctx, request, opts...)
		return err
	}

	return resp, backoff.Retry(op, c.policy, c.isRetryable)
}
