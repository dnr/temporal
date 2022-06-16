func (c *metricClient) QueryWorkflow(
	ctx context.Context,
	request *workflowservice.QueryWorkflowRequest,
	opts ...grpc.CallOption,
) (*workflowservice.QueryWorkflowResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientQueryWorkflowScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientQueryWorkflowScope, metrics.ClientLatency)
	resp, err := c.client.QueryWorkflow(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientQueryWorkflowScope, metrics.ClientFailures)
	}
	return resp, err
}
