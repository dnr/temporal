func (c *metricClient) RespondActivityTaskCanceled(
	ctx context.Context,
	request *historyservice.RespondActivityTaskCanceledRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RespondActivityTaskCanceledResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRespondActivityTaskCanceledScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RespondActivityTaskCanceled(ctx, request, opts...)
}
