func (c *metricClient) GetSearchAttributes(
	ctx context.Context,
	request *adminservice.GetSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetSearchAttributesResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientGetSearchAttributesScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientGetSearchAttributesScope, metrics.ClientLatency)
	resp, err := c.client.GetSearchAttributes(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientGetSearchAttributesScope, metrics.ClientFailures)
	}
	return resp, err
}
