func (c *retryableClient) RespondActivityTaskFailedById(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskFailedByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskFailedByIdResponse, error) {
	var resp *workflowservice.RespondActivityTaskFailedByIdResponse
	op := func() error {
		var err error
		resp, err = c.client.RespondActivityTaskFailedById(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
