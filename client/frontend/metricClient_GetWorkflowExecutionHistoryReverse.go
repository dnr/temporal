func (c *metricClient) GetWorkflowExecutionHistoryReverse(
	ctx context.Context,
	request *workflowservice.GetWorkflowExecutionHistoryReverseRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetWorkflowExecutionHistoryReverseResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientGetWorkflowExecutionHistoryReverseScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientGetWorkflowExecutionHistoryReverseScope, metrics.ClientLatency)
	resp, err := c.client.GetWorkflowExecutionHistoryReverse(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientGetWorkflowExecutionHistoryReverseScope, metrics.ClientFailures)
	}
	return resp, err
}
