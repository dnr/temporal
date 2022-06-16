func (c *metricClient) ListTaskQueuePartitions(
	ctx context.Context,
	request *workflowservice.ListTaskQueuePartitionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListTaskQueuePartitionsResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientListTaskQueuePartitionsScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientListTaskQueuePartitionsScope, metrics.ClientLatency)
	resp, err := c.client.ListTaskQueuePartitions(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientListTaskQueuePartitionsScope, metrics.ClientFailures)
	}
	return resp, err
}
