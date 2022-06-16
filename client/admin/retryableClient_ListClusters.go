func (c *retryableClient) ListClusters(
	ctx context.Context,
	request *adminservice.ListClustersRequest,
	opts ...grpc.CallOption,
) (*adminservice.ListClustersResponse, error) {
	var resp *adminservice.ListClustersResponse
	op := func() error {
		var err error
		resp, err = c.client.ListClusters(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
