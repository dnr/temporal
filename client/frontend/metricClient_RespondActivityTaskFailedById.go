func (c *metricClient) RespondActivityTaskFailedById(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskFailedByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskFailedByIdResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientRespondActivityTaskFailedByIdScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientRespondActivityTaskFailedByIdScope, metrics.ClientLatency)
	resp, err := c.client.RespondActivityTaskFailedById(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientRespondActivityTaskFailedByIdScope, metrics.ClientFailures)
	}
	return resp, err
}
