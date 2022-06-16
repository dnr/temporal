func (c *retryableClient) GetTaskQueueTasks(
	ctx context.Context,
	request *adminservice.GetTaskQueueTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetTaskQueueTasksResponse, error) {

	var resp *adminservice.GetTaskQueueTasksResponse
	op := func() error {
		var err error
		resp, err = c.client.GetTaskQueueTasks(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
