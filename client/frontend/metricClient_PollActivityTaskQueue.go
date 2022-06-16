func (c *metricClient) PollActivityTaskQueue(
	ctx context.Context,
	request *workflowservice.PollActivityTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PollActivityTaskQueueResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientPollActivityTaskQueueScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientPollActivityTaskQueueScope, metrics.ClientLatency)
	resp, err := c.client.PollActivityTaskQueue(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientPollActivityTaskQueueScope, metrics.ClientFailures)
	}
	return resp, err
}
