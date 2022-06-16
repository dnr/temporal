func (c *metricClient) ReapplyEvents(
	context context.Context,
	request *historyservice.ReapplyEventsRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.ReapplyEventsResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientReapplyEventsScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.ReapplyEvents(context, request, opts...)
}
