func (c *metricClient) CloseShard(
	ctx context.Context,
	request *adminservice.CloseShardRequest,
	opts ...grpc.CallOption,
) (*adminservice.CloseShardResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientCloseShardScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientCloseShardScope, metrics.ClientLatency)
	resp, err := c.client.CloseShard(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientCloseShardScope, metrics.ClientFailures)
	}
	return resp, err
}
