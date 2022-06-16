func (c *metricClient) TerminateWorkflowExecution(
	ctx context.Context,
	request *workflowservice.TerminateWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.TerminateWorkflowExecutionResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientTerminateWorkflowExecutionScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientTerminateWorkflowExecutionScope, metrics.ClientLatency)
	resp, err := c.client.TerminateWorkflowExecution(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientTerminateWorkflowExecutionScope, metrics.ClientFailures)
	}
	return resp, err
}
