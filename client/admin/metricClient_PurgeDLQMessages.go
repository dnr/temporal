func (c *metricClient) PurgeDLQMessages(
	ctx context.Context,
	request *adminservice.PurgeDLQMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.PurgeDLQMessagesResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientPurgeDLQMessagesScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientPurgeDLQMessagesScope, metrics.ClientLatency)
	resp, err := c.client.PurgeDLQMessages(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientPurgeDLQMessagesScope, metrics.ClientFailures)
	}
	return resp, err
}
