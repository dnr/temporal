func (c *metricClient) MergeDLQMessages(
	ctx context.Context,
	request *adminservice.MergeDLQMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.MergeDLQMessagesResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientMergeDLQMessagesScope, metrics.ClientRequests)
	sw := c.metricsClient.StartTimer(metrics.AdminClientMergeDLQMessagesScope, metrics.ClientLatency)
	resp, err := c.client.MergeDLQMessages(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientMergeDLQMessagesScope, metrics.ClientFailures)
	}
	return resp, err
}
