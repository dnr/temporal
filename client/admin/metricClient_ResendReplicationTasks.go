func (c *metricClient) ResendReplicationTasks(
	ctx context.Context,
	request *adminservice.ResendReplicationTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.ResendReplicationTasksResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientResendReplicationTasksScope, metrics.ClientRequests)
	sw := c.metricsClient.StartTimer(metrics.AdminClientResendReplicationTasksScope, metrics.ClientLatency)
	resp, err := c.client.ResendReplicationTasks(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientResendReplicationTasksScope, metrics.ClientFailures)
	}
	return resp, err
}
