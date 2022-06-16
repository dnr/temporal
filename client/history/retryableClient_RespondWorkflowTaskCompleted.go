func (c *retryableClient) RespondWorkflowTaskCompleted(
	ctx context.Context,
	request *historyservice.RespondWorkflowTaskCompletedRequest,
	opts ...grpc.CallOption) (*historyservice.RespondWorkflowTaskCompletedResponse, error) {

	var resp *historyservice.RespondWorkflowTaskCompletedResponse
	op := func() error {
		var err error
		resp, err = c.client.RespondWorkflowTaskCompleted(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
