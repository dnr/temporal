func (c *retryableClient) AddActivityTask(
	ctx context.Context,
	request *matchingservice.AddActivityTaskRequest,
	opts ...grpc.CallOption,
) (*matchingservice.AddActivityTaskResponse, error) {
	var resp *matchingservice.AddActivityTaskResponse
	op := func() error {
		var err error
		resp, err = c.client.AddActivityTask(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
