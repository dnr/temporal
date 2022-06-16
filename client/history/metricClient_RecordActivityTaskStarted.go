func (c *metricClient) RecordActivityTaskStarted(
	context context.Context,
	request *historyservice.RecordActivityTaskStartedRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RecordActivityTaskStartedResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRecordActivityTaskStartedScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RecordActivityTaskStarted(context, request, opts...)
}
