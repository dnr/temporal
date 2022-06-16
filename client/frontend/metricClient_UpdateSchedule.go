func (c *metricClient) UpdateSchedule(
	ctx context.Context,
	request *workflowservice.UpdateScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.UpdateScheduleResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientUpdateScheduleScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientUpdateScheduleScope, metrics.ClientLatency)
	resp, err := c.client.UpdateSchedule(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientUpdateScheduleScope, metrics.ClientFailures)
	}
	return resp, err
}
