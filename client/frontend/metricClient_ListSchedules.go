func (c *metricClient) ListSchedules(
	ctx context.Context,
	request *workflowservice.ListSchedulesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListSchedulesResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientListSchedulesScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientListSchedulesScope, metrics.ClientLatency)
	resp, err := c.client.ListSchedules(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientListSchedulesScope, metrics.ClientFailures)
	}
	return resp, err
}
