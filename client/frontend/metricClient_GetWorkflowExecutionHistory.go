func (c *metricClient) GetWorkflowExecutionHistory(
	ctx context.Context,
	request *workflowservice.GetWorkflowExecutionHistoryRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetWorkflowExecutionHistoryResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientGetWorkflowExecutionHistoryScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientGetWorkflowExecutionHistoryScope, metrics.ClientLatency)
	resp, err := c.client.GetWorkflowExecutionHistory(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientGetWorkflowExecutionHistoryScope, metrics.ClientFailures)
	}
	return resp, err
}
