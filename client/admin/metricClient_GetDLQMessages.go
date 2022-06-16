func (c *metricClient) GetDLQMessages(
	ctx context.Context,
	request *adminservice.GetDLQMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetDLQMessagesResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientGetDLQMessagesScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientGetDLQMessagesScope, metrics.ClientLatency)
	resp, err := c.client.GetDLQMessages(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientGetDLQMessagesScope, metrics.ClientFailures)
	}
	return resp, err
}
