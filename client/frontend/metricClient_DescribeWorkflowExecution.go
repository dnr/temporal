func (c *metricClient) DescribeWorkflowExecution(
	ctx context.Context,
	request *workflowservice.DescribeWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeWorkflowExecutionResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientDescribeWorkflowExecutionScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientDescribeWorkflowExecutionScope, metrics.ClientLatency)
	resp, err := c.client.DescribeWorkflowExecution(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientDescribeWorkflowExecutionScope, metrics.ClientFailures)
	}
	return resp, err
}
