func (c *retryableClient) RespondWorkflowTaskFailed(
	ctx context.Context,
	request *historyservice.RespondWorkflowTaskFailedRequest,
	opts ...grpc.CallOption) (*historyservice.RespondWorkflowTaskFailedResponse, error) {

	var resp *historyservice.RespondWorkflowTaskFailedResponse
	op := func() error {
		var err error
		resp, err = c.client.RespondWorkflowTaskFailed(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
