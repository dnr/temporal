func (c *retryableClient) RespondQueryTaskCompleted(
	ctx context.Context,
	request *matchingservice.RespondQueryTaskCompletedRequest,
	opts ...grpc.CallOption,
) (*matchingservice.RespondQueryTaskCompletedResponse, error) {
	var resp *matchingservice.RespondQueryTaskCompletedResponse
	op := func() error {
		var err error
		resp, err = c.client.RespondQueryTaskCompleted(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
