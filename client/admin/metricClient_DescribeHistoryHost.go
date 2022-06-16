func (c *metricClient) DescribeHistoryHost(
	ctx context.Context,
	request *adminservice.DescribeHistoryHostRequest,
	opts ...grpc.CallOption,
) (*adminservice.DescribeHistoryHostResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientDescribeHistoryHostScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientDescribeHistoryHostScope, metrics.ClientLatency)
	resp, err := c.client.DescribeHistoryHost(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientDescribeHistoryHostScope, metrics.ClientFailures)
	}
	return resp, err
}
