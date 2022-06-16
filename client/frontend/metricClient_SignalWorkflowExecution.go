func (c *metricClient) SignalWorkflowExecution(
	ctx context.Context,
	request *workflowservice.SignalWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.SignalWorkflowExecutionResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientSignalWorkflowExecutionScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientSignalWorkflowExecutionScope, metrics.ClientLatency)
	resp, err := c.client.SignalWorkflowExecution(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientSignalWorkflowExecutionScope, metrics.ClientFailures)
	}
	return resp, err
}
