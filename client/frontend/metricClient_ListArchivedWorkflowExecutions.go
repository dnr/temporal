func (c *metricClient) ListArchivedWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.ListArchivedWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListArchivedWorkflowExecutionsResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientListArchivedWorkflowExecutionsScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientListArchivedWorkflowExecutionsScope, metrics.ClientLatency)
	resp, err := c.client.ListArchivedWorkflowExecutions(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientListArchivedWorkflowExecutionsScope, metrics.ClientFailures)
	}
	return resp, err
}
