func (c *retryableClient) VerifyFirstWorkflowTaskScheduled(
	ctx context.Context,
	request *historyservice.VerifyFirstWorkflowTaskScheduledRequest,
	opts ...grpc.CallOption,
) (*historyservice.VerifyFirstWorkflowTaskScheduledResponse, error) {

	var resp *historyservice.VerifyFirstWorkflowTaskScheduledResponse
	op := func() error {
		var err error
		resp, err = c.client.VerifyFirstWorkflowTaskScheduled(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
