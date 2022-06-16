func (c *metricClient) ListClusters(
	ctx context.Context,
	request *adminservice.ListClustersRequest,
	opts ...grpc.CallOption,
) (*adminservice.ListClustersResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientListClustersScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientListClustersScope, metrics.ClientLatency)
	resp, err := c.client.ListClusters(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientListClustersScope, metrics.ClientFailures)
	}
	return resp, err
}
