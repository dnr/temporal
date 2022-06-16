func (c *metricClient) DeleteWorkflowExecution(
	ctx context.Context,
	request *adminservice.DeleteWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*adminservice.DeleteWorkflowExecutionResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientDeleteWorkflowExecutionScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientDeleteWorkflowExecutionScope, metrics.ClientLatency)
	resp, err := c.client.DeleteWorkflowExecution(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientDeleteWorkflowExecutionScope, metrics.ClientFailures)
	}
	return resp, err
}
