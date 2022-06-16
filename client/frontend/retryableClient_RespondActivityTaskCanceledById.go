func (c *retryableClient) RespondActivityTaskCanceledById(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskCanceledByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskCanceledByIdResponse, error) {
	var resp *workflowservice.RespondActivityTaskCanceledByIdResponse
	op := func() error {
		var err error
		resp, err = c.client.RespondActivityTaskCanceledById(ctx, request, opts...)
		return err
	}

	return resp, backoff.Retry(op, c.policy, c.isRetryable)
}
