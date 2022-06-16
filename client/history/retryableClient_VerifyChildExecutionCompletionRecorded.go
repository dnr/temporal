func (c *retryableClient) VerifyChildExecutionCompletionRecorded(
	ctx context.Context,
	request *historyservice.VerifyChildExecutionCompletionRecordedRequest,
	opts ...grpc.CallOption,
) (*historyservice.VerifyChildExecutionCompletionRecordedResponse, error) {
	var resp *historyservice.VerifyChildExecutionCompletionRecordedResponse
	op := func() error {
		var err error
		resp, err = c.client.VerifyChildExecutionCompletionRecorded(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
