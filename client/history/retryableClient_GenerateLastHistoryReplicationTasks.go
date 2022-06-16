func (c *retryableClient) GenerateLastHistoryReplicationTasks(
	ctx context.Context,
	request *historyservice.GenerateLastHistoryReplicationTasksRequest,
	opts ...grpc.CallOption,
) (*historyservice.GenerateLastHistoryReplicationTasksResponse, error) {

	var resp *historyservice.GenerateLastHistoryReplicationTasksResponse
	op := func() error {
		var err error
		resp, err = c.client.GenerateLastHistoryReplicationTasks(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
