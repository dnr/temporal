func (c *retryableClient) RecordChildExecutionCompleted(
	ctx context.Context,
	request *historyservice.RecordChildExecutionCompletedRequest,
	opts ...grpc.CallOption) (*historyservice.RecordChildExecutionCompletedResponse, error) {

	var resp *historyservice.RecordChildExecutionCompletedResponse
	op := func() error {
		var err error
		resp, err = c.client.RecordChildExecutionCompleted(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
