func (c *retryableClient) ResendReplicationTasks(
	ctx context.Context,
	request *adminservice.ResendReplicationTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.ResendReplicationTasksResponse, error) {

	var resp *adminservice.ResendReplicationTasksResponse
	op := func() error {
		var err error
		resp, err = c.client.ResendReplicationTasks(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
