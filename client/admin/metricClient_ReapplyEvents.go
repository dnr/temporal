func (c *metricClient) ReapplyEvents(
	ctx context.Context,
	request *adminservice.ReapplyEventsRequest,
	opts ...grpc.CallOption,
) (*adminservice.ReapplyEventsResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientReapplyEventsScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientReapplyEventsScope, metrics.ClientLatency)
	resp, err := c.client.ReapplyEvents(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientReapplyEventsScope, metrics.ClientFailures)
	}
	return resp, err
}
