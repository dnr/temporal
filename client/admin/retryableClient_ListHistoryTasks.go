func (c *retryableClient) ListHistoryTasks(
	ctx context.Context,
	request *adminservice.ListHistoryTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.ListHistoryTasksResponse, error) {

	var resp *adminservice.ListHistoryTasksResponse
	op := func() error {
		var err error
		resp, err = c.client.ListHistoryTasks(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
