func (c *metricClient) DescribeNamespace(
	ctx context.Context,
	request *workflowservice.DescribeNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeNamespaceResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientDescribeNamespaceScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientDescribeNamespaceScope, metrics.ClientLatency)
	resp, err := c.client.DescribeNamespace(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientDescribeNamespaceScope, metrics.ClientFailures)
	}
	return resp, err
}
