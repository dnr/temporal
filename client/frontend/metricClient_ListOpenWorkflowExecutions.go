func (c *metricClient) ListOpenWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.ListOpenWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListOpenWorkflowExecutionsResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientListOpenWorkflowExecutionsScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientListOpenWorkflowExecutionsScope, metrics.ClientLatency)
	resp, err := c.client.ListOpenWorkflowExecutions(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientListOpenWorkflowExecutionsScope, metrics.ClientFailures)
	}
	return resp, err
}
