func (c *metricClient) VerifyChildExecutionCompletionRecorded(
	context context.Context,
	request *historyservice.VerifyChildExecutionCompletionRecordedRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.VerifyChildExecutionCompletionRecordedResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientVerifyChildExecutionCompletionRecordedScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.VerifyChildExecutionCompletionRecorded(context, request, opts...)
}
