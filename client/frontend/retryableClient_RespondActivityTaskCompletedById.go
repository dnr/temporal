func (c *retryableClient) RespondActivityTaskCompletedById(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskCompletedByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskCompletedByIdResponse, error) {
	var resp *workflowservice.RespondActivityTaskCompletedByIdResponse
	op := func() error {
		var err error
		resp, err = c.client.RespondActivityTaskCompletedById(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
