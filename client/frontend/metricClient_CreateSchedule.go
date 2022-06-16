func (c *metricClient) CreateSchedule(
	ctx context.Context,
	request *workflowservice.CreateScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.CreateScheduleResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientCreateScheduleScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientCreateScheduleScope, metrics.ClientLatency)
	resp, err := c.client.CreateSchedule(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientCreateScheduleScope, metrics.ClientFailures)
	}
	return resp, err
}
