func (c *retryableClient) PollWorkflowTaskQueue(
	ctx context.Context,
	request *workflowservice.PollWorkflowTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PollWorkflowTaskQueueResponse, error) {
	var resp *workflowservice.PollWorkflowTaskQueueResponse
	op := func() error {
		var err error
		resp, err = c.client.PollWorkflowTaskQueue(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
