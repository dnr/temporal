func (c *metricClient) VerifyFirstWorkflowTaskScheduled(
	context context.Context,
	request *historyservice.VerifyFirstWorkflowTaskScheduledRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.VerifyFirstWorkflowTaskScheduledResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientVerifyFirstWorkflowTaskScheduled)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.VerifyFirstWorkflowTaskScheduled(context, request, opts...)
}
