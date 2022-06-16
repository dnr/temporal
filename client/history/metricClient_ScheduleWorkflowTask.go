func (c *metricClient) ScheduleWorkflowTask(
	ctx context.Context,
	request *historyservice.ScheduleWorkflowTaskRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.ScheduleWorkflowTaskResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientScheduleWorkflowTaskScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.ScheduleWorkflowTask(ctx, request, opts...)
}
