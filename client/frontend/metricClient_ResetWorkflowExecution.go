func (c *metricClient) ResetWorkflowExecution(
	ctx context.Context,
	request *workflowservice.ResetWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ResetWorkflowExecutionResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientResetWorkflowExecutionScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientResetWorkflowExecutionScope, metrics.ClientLatency)
	resp, err := c.client.ResetWorkflowExecution(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientResetWorkflowExecutionScope, metrics.ClientFailures)
	}
	return resp, err
}
