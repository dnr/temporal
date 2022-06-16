func (c *metricClient) GetSystemInfo(
	ctx context.Context,
	request *workflowservice.GetSystemInfoRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetSystemInfoResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientGetSystemInfoScope, metrics.ClientRequests)
	sw := c.metricsClient.StartTimer(metrics.FrontendClientGetSystemInfoScope, metrics.ClientLatency)
	resp, err := c.client.GetSystemInfo(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientGetSystemInfoScope, metrics.ClientFailures)
	}
	return resp, err
}
