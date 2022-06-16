func (c *metricClient) ScanWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.ScanWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ScanWorkflowExecutionsResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientScanWorkflowExecutionsScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientScanWorkflowExecutionsScope, metrics.ClientLatency)
	resp, err := c.client.ScanWorkflowExecutions(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientScanWorkflowExecutionsScope, metrics.ClientFailures)
	}
	return resp, err
}
