func (c *metricClient) RespondWorkflowTaskFailed(
	ctx context.Context,
	request *workflowservice.RespondWorkflowTaskFailedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondWorkflowTaskFailedResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientRespondWorkflowTaskFailedScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientRespondWorkflowTaskFailedScope, metrics.ClientLatency)
	resp, err := c.client.RespondWorkflowTaskFailed(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientRespondWorkflowTaskFailedScope, metrics.ClientFailures)
	}
	return resp, err
}
