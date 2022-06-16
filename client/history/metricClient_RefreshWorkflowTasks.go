func (c *metricClient) RefreshWorkflowTasks(
	ctx context.Context,
	request *historyservice.RefreshWorkflowTasksRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RefreshWorkflowTasksResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRefreshWorkflowTasksScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RefreshWorkflowTasks(ctx, request, opts...)
}
