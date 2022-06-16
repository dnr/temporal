func (c *metricClient) RespondActivityTaskCompleted(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskCompletedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskCompletedResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientRespondActivityTaskCompletedScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientRespondActivityTaskCompletedScope, metrics.ClientLatency)
	resp, err := c.client.RespondActivityTaskCompleted(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientRespondActivityTaskCompletedScope, metrics.ClientFailures)
	}
	return resp, err
}
