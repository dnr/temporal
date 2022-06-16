func (c *metricClient) RemoveRemoteCluster(
	ctx context.Context,
	request *adminservice.RemoveRemoteClusterRequest,
	opts ...grpc.CallOption,
) (*adminservice.RemoveRemoteClusterResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientRemoveRemoteClusterScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientRemoveRemoteClusterScope, metrics.ClientLatency)
	resp, err := c.client.RemoveRemoteCluster(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientRemoveRemoteClusterScope, metrics.ClientFailures)
	}
	return resp, err
}
