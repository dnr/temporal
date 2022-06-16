func (c *metricClient) GetShard(
	ctx context.Context,
	request *adminservice.GetShardRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetShardResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientGetShardScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientGetShardScope, metrics.ClientLatency)
	resp, err := c.client.GetShard(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientGetShardScope, metrics.ClientFailures)
	}
	return resp, err
}
