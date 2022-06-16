func (c *retryableClient) RespondActivityTaskFailed(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskFailedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskFailedResponse, error) {
	var resp *workflowservice.RespondActivityTaskFailedResponse
	op := func() error {
		var err error
		resp, err = c.client.RespondActivityTaskFailed(ctx, request, opts...)
		return err
	}

	return resp, backoff.Retry(op, c.policy, c.isRetryable)
}
