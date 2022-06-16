func (c *metricClient) GetTaskQueueTasks(
	ctx context.Context,
	request *adminservice.GetTaskQueueTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetTaskQueueTasksResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientResendReplicationTasksScope, metrics.ClientRequests)
	sw := c.metricsClient.StartTimer(metrics.AdminClientResendReplicationTasksScope, metrics.ClientLatency)
	resp, err := c.client.GetTaskQueueTasks(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientResendReplicationTasksScope, metrics.ClientFailures)
	}
	return resp, err
}
