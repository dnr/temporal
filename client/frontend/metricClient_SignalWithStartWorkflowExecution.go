func (c *metricClient) SignalWithStartWorkflowExecution(
	ctx context.Context,
	request *workflowservice.SignalWithStartWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.SignalWithStartWorkflowExecutionResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientSignalWithStartWorkflowExecutionScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientSignalWithStartWorkflowExecutionScope, metrics.ClientLatency)
	resp, err := c.client.SignalWithStartWorkflowExecution(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientSignalWithStartWorkflowExecutionScope, metrics.ClientFailures)
	}
	return resp, err
}
