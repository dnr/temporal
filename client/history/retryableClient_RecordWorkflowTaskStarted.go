func (c *retryableClient) RecordWorkflowTaskStarted(
	ctx context.Context,
	request *historyservice.RecordWorkflowTaskStartedRequest,
	opts ...grpc.CallOption,
) (*historyservice.RecordWorkflowTaskStartedResponse, error) {
	var resp *historyservice.RecordWorkflowTaskStartedResponse
	op := func() error {
		var err error
		resp, err = c.client.RecordWorkflowTaskStarted(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
