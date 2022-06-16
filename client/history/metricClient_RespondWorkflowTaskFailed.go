func (c *metricClient) RespondWorkflowTaskFailed(
	context context.Context,
	request *historyservice.RespondWorkflowTaskFailedRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RespondWorkflowTaskFailedResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRespondWorkflowTaskFailedScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RespondWorkflowTaskFailed(context, request, opts...)
}
