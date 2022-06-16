func (c *metricClient) DeprecateNamespace(
	ctx context.Context,
	request *workflowservice.DeprecateNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DeprecateNamespaceResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientDeprecateNamespaceScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientDeprecateNamespaceScope, metrics.ClientLatency)
	resp, err := c.client.DeprecateNamespace(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientDeprecateNamespaceScope, metrics.ClientFailures)
	}
	return resp, err
}
