func (c *metricClient) AddOrUpdateRemoteCluster(
	ctx context.Context,
	request *adminservice.AddOrUpdateRemoteClusterRequest,
	opts ...grpc.CallOption,
) (*adminservice.AddOrUpdateRemoteClusterResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientAddOrUpdateRemoteClusterScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientAddOrUpdateRemoteClusterScope, metrics.ClientLatency)
	resp, err := c.client.AddOrUpdateRemoteCluster(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientAddOrUpdateRemoteClusterScope, metrics.ClientFailures)
	}
	return resp, err
}
