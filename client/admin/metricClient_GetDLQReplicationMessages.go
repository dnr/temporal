func (c *metricClient) GetDLQReplicationMessages(
	ctx context.Context,
	request *adminservice.GetDLQReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetDLQReplicationMessagesResponse, error) {
	c.metricsClient.IncCounter(metrics.FrontendClientGetDLQReplicationTasksScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientGetDLQReplicationTasksScope, metrics.ClientLatency)
	resp, err := c.client.GetDLQReplicationMessages(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientGetDLQReplicationTasksScope, metrics.ClientFailures)
	}
	return resp, err
}
