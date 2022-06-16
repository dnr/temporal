func (c *metricClient) ListHistoryTasks(
	ctx context.Context,
	request *adminservice.ListHistoryTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.ListHistoryTasksResponse, error) {
	c.metricsClient.IncCounter(metrics.AdminClientListHistoryTasksScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientListHistoryTasksScope, metrics.ClientLatency)
	resp, err := c.client.ListHistoryTasks(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientListHistoryTasksScope, metrics.ClientFailures)
	}
	return resp, err
}
