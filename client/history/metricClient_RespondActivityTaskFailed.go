func (c *metricClient) RespondActivityTaskFailed(
	context context.Context,
	request *historyservice.RespondActivityTaskFailedRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RespondActivityTaskFailedResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRespondActivityTaskFailedScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RespondActivityTaskFailed(context, request, opts...)
}
