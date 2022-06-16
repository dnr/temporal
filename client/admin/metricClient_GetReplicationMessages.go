func (c *metricClient) GetReplicationMessages(
	ctx context.Context,
	request *adminservice.GetReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetReplicationMessagesResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientGetReplicationMessagesScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientGetReplicationMessagesScope, metrics.ClientLatency)
	resp, err := c.client.GetReplicationMessages(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientGetReplicationMessagesScope, metrics.ClientFailures)
	}
	return resp, err
}
