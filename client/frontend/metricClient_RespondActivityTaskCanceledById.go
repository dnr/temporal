func (c *metricClient) RespondActivityTaskCanceledById(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskCanceledByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskCanceledByIdResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientRespondActivityTaskCanceledByIdScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientRespondActivityTaskCanceledByIdScope, metrics.ClientLatency)
	resp, err := c.client.RespondActivityTaskCanceledById(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientRespondActivityTaskCanceledByIdScope, metrics.ClientFailures)
	}
	return resp, err
}
