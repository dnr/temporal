func (c *metricClient) ReapplyEvents(
	ctx context.Context,
	request *adminservice.ReapplyEventsRequest,
	opts ...grpc.CallOption,
) (*adminservice.ReapplyEventsResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientReapplyEventsScope, metrics.ClientRequests)
	sw := c.metricsClient.StartTimer(metrics.FrontendClientReapplyEventsScope, metrics.ClientLatency)
	resp, err := c.client.ReapplyEvents(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientReapplyEventsScope, metrics.ClientFailures)
	}
	return resp, err
}
