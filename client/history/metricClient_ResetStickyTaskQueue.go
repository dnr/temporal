func (c *metricClient) ResetStickyTaskQueue(
	ctx context.Context,
	request *historyservice.ResetStickyTaskQueueRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.ResetStickyTaskQueueResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientResetStickyTaskQueueScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.ResetStickyTaskQueue(ctx, request, opts...)
}
