func (c *metricClient) RequestCancelWorkflowExecution(
	ctx context.Context,
	request *workflowservice.RequestCancelWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RequestCancelWorkflowExecutionResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientRequestCancelWorkflowExecutionScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientRequestCancelWorkflowExecutionScope, metrics.ClientLatency)
	resp, err := c.client.RequestCancelWorkflowExecution(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientRequestCancelWorkflowExecutionScope, metrics.ClientFailures)
	}
	return resp, err
}
