func (c *retryableClient) AddActivityTask(
	ctx context.Context,
	addRequest *matchingservice.AddActivityTaskRequest,
	opts ...grpc.CallOption) (*matchingservice.AddActivityTaskResponse, error) {

	var resp *matchingservice.AddActivityTaskResponse
	op := func() error {
		var err error
		resp, err = c.client.AddActivityTask(ctx, addRequest, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
