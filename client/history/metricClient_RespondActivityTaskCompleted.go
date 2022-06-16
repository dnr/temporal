func (c *metricClient) RespondActivityTaskCompleted(
	ctx context.Context,
	request *historyservice.RespondActivityTaskCompletedRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RespondActivityTaskCompletedResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRespondActivityTaskCompletedScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RespondActivityTaskCompleted(ctx, request, opts...)
}
