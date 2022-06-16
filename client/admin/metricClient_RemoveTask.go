func (c *metricClient) RemoveTask(
	ctx context.Context,
	request *adminservice.RemoveTaskRequest,
	opts ...grpc.CallOption,
) (*adminservice.RemoveTaskResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientCloseShardScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientCloseShardScope, metrics.ClientLatency)
	resp, err := c.client.RemoveTask(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientCloseShardScope, metrics.ClientFailures)
	}
	return resp, err
}
