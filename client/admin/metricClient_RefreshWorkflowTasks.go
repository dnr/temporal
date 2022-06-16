func (c *metricClient) RefreshWorkflowTasks(
	ctx context.Context,
	request *adminservice.RefreshWorkflowTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.RefreshWorkflowTasksResponse, error) {

	c.metricsClient.IncCounter(metrics.AdminClientRefreshWorkflowTasksScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientRefreshWorkflowTasksScope, metrics.ClientLatency)
	resp, err := c.client.RefreshWorkflowTasks(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientRefreshWorkflowTasksScope, metrics.ClientFailures)
	}
	return resp, err
}
