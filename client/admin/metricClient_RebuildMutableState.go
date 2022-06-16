func (c *metricClient) RebuildMutableState(
	ctx context.Context,
	request *adminservice.RebuildMutableStateRequest,
	opts ...grpc.CallOption,
) (*adminservice.RebuildMutableStateResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientRebuildMutableStateScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientRebuildMutableStateScope, metrics.ClientLatency)
	resp, err := c.client.RebuildMutableState(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientRebuildMutableStateScope, metrics.ClientFailures)
	}
	return resp, err
}
