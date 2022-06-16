func (c *metricClient) DescribeCluster(
	ctx context.Context,
	request *adminservice.DescribeClusterRequest,
	opts ...grpc.CallOption,
) (*adminservice.DescribeClusterResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientDescribeClusterScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientDescribeClusterScope, metrics.ClientLatency)
	resp, err := c.client.DescribeCluster(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientDescribeClusterScope, metrics.ClientFailures)
	}
	return resp, err
}
