func (c *metricClient) StartWorkflowExecution(
	ctx context.Context,
	request *workflowservice.StartWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.StartWorkflowExecutionResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientStartWorkflowExecutionScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientStartWorkflowExecutionScope, metrics.ClientLatency)
	resp, err := c.client.StartWorkflowExecution(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientStartWorkflowExecutionScope, metrics.ClientFailures)
	}
	return resp, err
}
