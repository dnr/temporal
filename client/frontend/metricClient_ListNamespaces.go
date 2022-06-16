func (c *metricClient) ListNamespaces(
	ctx context.Context,
	request *workflowservice.ListNamespacesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListNamespacesResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientListNamespacesScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientListNamespacesScope, metrics.ClientLatency)
	resp, err := c.client.ListNamespaces(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientListNamespacesScope, metrics.ClientFailures)
	}
	return resp, err
}
