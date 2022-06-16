func (c *metricClient) GetSearchAttributes(
	ctx context.Context,
	request *workflowservice.GetSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetSearchAttributesResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientGetSearchAttributesScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientGetSearchAttributesScope, metrics.ClientLatency)
	resp, err := c.client.GetSearchAttributes(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientGetSearchAttributesScope, metrics.ClientFailures)
	}
	return resp, err
}
