func (c *metricClient) RecordChildExecutionCompleted(
	context context.Context,
	request *historyservice.RecordChildExecutionCompletedRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RecordChildExecutionCompletedResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRecordChildExecutionCompletedScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RecordChildExecutionCompleted(context, request, opts...)
}
