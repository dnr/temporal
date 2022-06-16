func (c *metricClient) AddSearchAttributes(
	ctx context.Context,
	request *adminservice.AddSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*adminservice.AddSearchAttributesResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientAddSearchAttributesScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientAddSearchAttributesScope, metrics.ClientLatency)
	resp, err := c.client.AddSearchAttributes(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientAddSearchAttributesScope, metrics.ClientFailures)
	}
	return resp, err
}
