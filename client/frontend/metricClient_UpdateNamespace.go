func (c *metricClient) UpdateNamespace(
	ctx context.Context,
	request *workflowservice.UpdateNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.UpdateNamespaceResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientUpdateNamespaceScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientUpdateNamespaceScope, metrics.ClientLatency)
	resp, err := c.client.UpdateNamespace(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientUpdateNamespaceScope, metrics.ClientFailures)
	}
	return resp, err
}
