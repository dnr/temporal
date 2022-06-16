func (c *metricClient) RespondWorkflowTaskCompleted(
	context context.Context,
	request *historyservice.RespondWorkflowTaskCompletedRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RespondWorkflowTaskCompletedResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRespondWorkflowTaskCompletedScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RespondWorkflowTaskCompleted(context, request, opts...)
}
