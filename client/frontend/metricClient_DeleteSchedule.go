func (c *metricClient) DeleteSchedule(
	ctx context.Context,
	request *workflowservice.DeleteScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DeleteScheduleResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientDeleteScheduleScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientDeleteScheduleScope, metrics.ClientLatency)
	resp, err := c.client.DeleteSchedule(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientDeleteScheduleScope, metrics.ClientFailures)
	}
	return resp, err
}
