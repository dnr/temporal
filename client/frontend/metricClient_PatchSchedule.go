func (c *metricClient) PatchSchedule(
	ctx context.Context,
	request *workflowservice.PatchScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PatchScheduleResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientPatchScheduleScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientPatchScheduleScope, metrics.ClientLatency)
	resp, err := c.client.PatchSchedule(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientPatchScheduleScope, metrics.ClientFailures)
	}
	return resp, err
}
