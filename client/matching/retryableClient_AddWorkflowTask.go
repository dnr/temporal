func (c *retryableClient) AddWorkflowTask(
	ctx context.Context,
	addRequest *matchingservice.AddWorkflowTaskRequest,
	opts ...grpc.CallOption) (*matchingservice.AddWorkflowTaskResponse, error) {

	var resp *matchingservice.AddWorkflowTaskResponse
	op := func() error {
		var err error
		resp, err = c.client.AddWorkflowTask(ctx, addRequest, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
