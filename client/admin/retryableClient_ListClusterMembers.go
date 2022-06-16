func (c *retryableClient) ListClusterMembers(
	ctx context.Context,
	request *adminservice.ListClusterMembersRequest,
	opts ...grpc.CallOption,
) (*adminservice.ListClusterMembersResponse, error) {
	var resp *adminservice.ListClusterMembersResponse
	op := func() error {
		var err error
		resp, err = c.client.ListClusterMembers(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
