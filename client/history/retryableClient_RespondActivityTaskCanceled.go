func (c *retryableClient) RespondActivityTaskCanceled(
	ctx context.Context,
	request *historyservice.RespondActivityTaskCanceledRequest,
	opts ...grpc.CallOption) (*historyservice.RespondActivityTaskCanceledResponse, error) {

	var resp *historyservice.RespondActivityTaskCanceledResponse
	op := func() error {
		var err error
		resp, err = c.client.RespondActivityTaskCanceled(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
