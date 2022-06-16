func (c *retryableClient) CancelOutstandingPoll(
	ctx context.Context,
	request *matchingservice.CancelOutstandingPollRequest,
	opts ...grpc.CallOption) (*matchingservice.CancelOutstandingPollResponse, error) {

	var resp *matchingservice.CancelOutstandingPollResponse
	op := func() error {
		var err error
		resp, err = c.client.CancelOutstandingPoll(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
