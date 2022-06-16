func (c *metricClient) GetDLQReplicationMessages(
	ctx context.Context,
	request *adminservice.GetDLQReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetDLQReplicationMessagesResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientGetDLQReplicationMessagesScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientGetDLQReplicationMessagesScope, metrics.ClientLatency)
	resp, err := c.client.GetDLQReplicationMessages(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientGetDLQReplicationMessagesScope, metrics.ClientFailures)
	}
	return resp, err
}
