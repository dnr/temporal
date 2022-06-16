func (c *metricClient) RegisterNamespace(
	ctx context.Context,
	request *workflowservice.RegisterNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RegisterNamespaceResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientRegisterNamespaceScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientRegisterNamespaceScope, metrics.ClientLatency)
	resp, err := c.client.RegisterNamespace(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientRegisterNamespaceScope, metrics.ClientFailures)
	}
	return resp, err
}
