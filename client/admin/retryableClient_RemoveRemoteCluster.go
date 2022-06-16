func (c *retryableClient) RemoveRemoteCluster(
	ctx context.Context,
	request *adminservice.RemoveRemoteClusterRequest,
	opts ...grpc.CallOption,
) (*adminservice.RemoveRemoteClusterResponse, error) {
	var resp *adminservice.RemoveRemoteClusterResponse
	op := func() error {
		var err error
		resp, err = c.client.RemoveRemoteCluster(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
