func (c *metricClient) RespondActivityTaskFailed(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskFailedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskFailedResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientRespondActivityTaskFailedScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientRespondActivityTaskFailedScope, metrics.ClientLatency)
	resp, err := c.client.RespondActivityTaskFailed(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientRespondActivityTaskFailedScope, metrics.ClientFailures)
	}
	return resp, err
}
