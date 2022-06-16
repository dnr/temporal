func (c *metricClient) DescribeSchedule(
	ctx context.Context,
	request *workflowservice.DescribeScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeScheduleResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientDescribeScheduleScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientDescribeScheduleScope, metrics.ClientLatency)
	resp, err := c.client.DescribeSchedule(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientDescribeScheduleScope, metrics.ClientFailures)
	}
	return resp, err
}
