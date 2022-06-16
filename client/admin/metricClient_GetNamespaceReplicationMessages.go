func (c *metricClient) GetNamespaceReplicationMessages(
	ctx context.Context,
	request *adminservice.GetNamespaceReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetNamespaceReplicationMessagesResponse, error) {
	c.metricsClient.IncCounter(metrics.FrontendClientGetNamespaceReplicationTasksScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientGetNamespaceReplicationTasksScope, metrics.ClientLatency)
	resp, err := c.client.GetNamespaceReplicationMessages(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientGetNamespaceReplicationTasksScope, metrics.ClientFailures)
	}
	return resp, err
}
