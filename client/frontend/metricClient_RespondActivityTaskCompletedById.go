func (c *metricClient) RespondActivityTaskCompletedById(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskCompletedByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskCompletedByIdResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientRespondActivityTaskCompletedByIdScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientRespondActivityTaskCompletedByIdScope, metrics.ClientLatency)
	resp, err := c.client.RespondActivityTaskCompletedById(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientRespondActivityTaskCompletedByIdScope, metrics.ClientFailures)
	}
	return resp, err
}
