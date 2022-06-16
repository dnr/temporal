func (c *retryableClient) AddOrUpdateRemoteCluster(
	ctx context.Context,
	request *adminservice.AddOrUpdateRemoteClusterRequest,
	opts ...grpc.CallOption,
) (*adminservice.AddOrUpdateRemoteClusterResponse, error) {

	var resp *adminservice.AddOrUpdateRemoteClusterResponse
	op := func() error {
		var err error
		resp, err = c.client.AddOrUpdateRemoteCluster(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
