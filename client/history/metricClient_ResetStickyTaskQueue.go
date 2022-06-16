func (c *metricClient) ResetStickyTaskQueue(
	context context.Context,
	request *historyservice.ResetStickyTaskQueueRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.ResetStickyTaskQueueResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientResetStickyTaskQueueScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.ResetStickyTaskQueue(context, request, opts...)
}
