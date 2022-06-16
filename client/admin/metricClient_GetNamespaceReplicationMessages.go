func (c *metricClient) GetNamespaceReplicationMessages(
	ctx context.Context,
	request *adminservice.GetNamespaceReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetNamespaceReplicationMessagesResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientGetNamespaceReplicationMessagesScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientGetNamespaceReplicationMessagesScope, metrics.ClientLatency)
	resp, err := c.client.GetNamespaceReplicationMessages(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientGetNamespaceReplicationMessagesScope, metrics.ClientFailures)
	}
	return resp, err
}
