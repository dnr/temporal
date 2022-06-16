func (c *metricClient) ListClusterMembers(
	ctx context.Context,
	request *adminservice.ListClusterMembersRequest,
	opts ...grpc.CallOption,
) (*adminservice.ListClusterMembersResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientListClusterMembersScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientListClusterMembersScope, metrics.ClientLatency)
	resp, err := c.client.ListClusterMembers(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientListClusterMembersScope, metrics.ClientFailures)
	}
	return resp, err
}
