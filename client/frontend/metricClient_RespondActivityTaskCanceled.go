func (c *metricClient) RespondActivityTaskCanceled(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskCanceledRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskCanceledResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientRespondActivityTaskCanceledScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientRespondActivityTaskCanceledScope, metrics.ClientLatency)
	resp, err := c.client.RespondActivityTaskCanceled(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientRespondActivityTaskCanceledScope, metrics.ClientFailures)
	}
	return resp, err
}
