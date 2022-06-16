func (c *retryableClient) DescribeCluster(
	ctx context.Context,
	request *adminservice.DescribeClusterRequest,
	opts ...grpc.CallOption,
) (*adminservice.DescribeClusterResponse, error) {

	var resp *adminservice.DescribeClusterResponse
	op := func() error {
		var err error
		resp, err = c.client.DescribeCluster(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
