func (c *metricClient) GetClusterInfo(
	ctx context.Context,
	request *workflowservice.GetClusterInfoRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetClusterInfoResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientGetClusterInfoScope, metrics.ClientRequests)
	sw := c.metricsClient.StartTimer(metrics.FrontendClientGetClusterInfoScope, metrics.ClientLatency)
	resp, err := c.client.GetClusterInfo(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientGetClusterInfoScope, metrics.ClientFailures)
	}
	return resp, err
}
