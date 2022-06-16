func (c *metricClient) RespondWorkflowTaskCompleted(
	ctx context.Context,
	request *workflowservice.RespondWorkflowTaskCompletedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondWorkflowTaskCompletedResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientRespondWorkflowTaskCompletedScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientRespondWorkflowTaskCompletedScope, metrics.ClientLatency)
	resp, err := c.client.RespondWorkflowTaskCompleted(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientRespondWorkflowTaskCompletedScope, metrics.ClientFailures)
	}
	return resp, err
}
