func (c *metricClient) DeleteWorkflowExecution(
	context context.Context,
	request *historyservice.DeleteWorkflowExecutionRequest,
	opts ...grpc.CallOption) (*historyservice.DeleteWorkflowExecutionResponse, error) {
	c.metricsClient.IncCounter(metrics.HistoryClientDeleteWorkflowExecutionScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.HistoryClientDeleteWorkflowExecutionScope, metrics.ClientLatency)
	resp, err := c.client.DeleteWorkflowExecution(context, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.HistoryClientDeleteWorkflowExecutionScope, metrics.ClientFailures)
	}

	return resp, err
}
