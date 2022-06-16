func (c *metricClient) RespondQueryTaskCompleted(
	ctx context.Context,
	request *workflowservice.RespondQueryTaskCompletedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondQueryTaskCompletedResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientRespondQueryTaskCompletedScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientRespondQueryTaskCompletedScope, metrics.ClientLatency)
	resp, err := c.client.RespondQueryTaskCompleted(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientRespondQueryTaskCompletedScope, metrics.ClientFailures)
	}
	return resp, err
}
