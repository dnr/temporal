func (c *metricClient) DescribeTaskQueue(
	ctx context.Context,
	request *workflowservice.DescribeTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeTaskQueueResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientDescribeTaskQueueScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientDescribeTaskQueueScope, metrics.ClientLatency)
	resp, err := c.client.DescribeTaskQueue(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientDescribeTaskQueueScope, metrics.ClientFailures)
	}
	return resp, err
}
