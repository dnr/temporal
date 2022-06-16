func (c *metricClient) RemoveSearchAttributes(
	ctx context.Context,
	request *adminservice.RemoveSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*adminservice.RemoveSearchAttributesResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientRemoveSearchAttributesScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientRemoveSearchAttributesScope, metrics.ClientLatency)
	resp, err := c.client.RemoveSearchAttributes(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientRemoveSearchAttributesScope, metrics.ClientFailures)
	}
	return resp, err
}
