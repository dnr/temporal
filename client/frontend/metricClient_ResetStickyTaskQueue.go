func (c *metricClient) ResetStickyTaskQueue(
	ctx context.Context,
	request *workflowservice.ResetStickyTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ResetStickyTaskQueueResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientResetStickyTaskQueueScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientResetStickyTaskQueueScope, metrics.ClientLatency)
	resp, err := c.client.ResetStickyTaskQueue(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientResetStickyTaskQueueScope, metrics.ClientFailures)
	}
	return resp, err
}
