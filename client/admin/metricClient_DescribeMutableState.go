func (c *metricClient) DescribeMutableState(
	ctx context.Context,
	request *adminservice.DescribeMutableStateRequest,
	opts ...grpc.CallOption,
) (*adminservice.DescribeMutableStateResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientDescribeMutableStateScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientDescribeMutableStateScope, metrics.ClientLatency)
	resp, err := c.client.DescribeMutableState(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientDescribeMutableStateScope, metrics.ClientFailures)
	}
	return resp, err
}
