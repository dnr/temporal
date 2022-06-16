func (c *metricClient) GetTaskQueueTasks(
	ctx context.Context,
	request *adminservice.GetTaskQueueTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetTaskQueueTasksResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientGetTaskQueueTasksScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientGetTaskQueueTasksScope, metrics.ClientLatency)
	resp, err := c.client.GetTaskQueueTasks(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientGetTaskQueueTasksScope, metrics.ClientFailures)
	}
	return resp, err
}
