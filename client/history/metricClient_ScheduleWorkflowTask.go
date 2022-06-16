func (c *metricClient) ScheduleWorkflowTask(
	context context.Context,
	request *historyservice.ScheduleWorkflowTaskRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.ScheduleWorkflowTaskResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientScheduleWorkflowTaskScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.ScheduleWorkflowTask(context, request, opts...)
}
