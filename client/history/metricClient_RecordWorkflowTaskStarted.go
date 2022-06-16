func (c *metricClient) RecordWorkflowTaskStarted(
	context context.Context,
	request *historyservice.RecordWorkflowTaskStartedRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RecordWorkflowTaskStartedResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRecordWorkflowTaskStartedScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RecordWorkflowTaskStarted(context, request, opts...)
}
