func (c *metricClient) ListScheduleMatchingTimes(
	ctx context.Context,
	request *workflowservice.ListScheduleMatchingTimesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListScheduleMatchingTimesResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientListScheduleMatchingTimesScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientListScheduleMatchingTimesScope, metrics.ClientLatency)
	resp, err := c.client.ListScheduleMatchingTimes(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientListScheduleMatchingTimesScope, metrics.ClientFailures)
	}
	return resp, err
}
