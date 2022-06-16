func (c *metricClient) ListClosedWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.ListClosedWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListClosedWorkflowExecutionsResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientListClosedWorkflowExecutionsScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientListClosedWorkflowExecutionsScope, metrics.ClientLatency)
	resp, err := c.client.ListClosedWorkflowExecutions(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientListClosedWorkflowExecutionsScope, metrics.ClientFailures)
	}
	return resp, err
}
