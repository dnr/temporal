func (c *metricClient) ListWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.ListWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListWorkflowExecutionsResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientListWorkflowExecutionsScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientListWorkflowExecutionsScope, metrics.ClientLatency)
	resp, err := c.client.ListWorkflowExecutions(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientListWorkflowExecutionsScope, metrics.ClientFailures)
	}
	return resp, err
}
