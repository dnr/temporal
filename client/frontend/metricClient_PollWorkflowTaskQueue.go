func (c *metricClient) PollWorkflowTaskQueue(
	ctx context.Context,
	request *workflowservice.PollWorkflowTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PollWorkflowTaskQueueResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientPollWorkflowTaskQueueScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientPollWorkflowTaskQueueScope, metrics.ClientLatency)
	resp, err := c.client.PollWorkflowTaskQueue(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientPollWorkflowTaskQueueScope, metrics.ClientFailures)
	}
	return resp, err
}
