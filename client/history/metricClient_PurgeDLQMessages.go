func (c *metricClient) PurgeDLQMessages(
	ctx context.Context,
	request *historyservice.PurgeDLQMessagesRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.PurgeDLQMessagesResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientPurgeDLQMessagesScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.PurgeDLQMessages(ctx, request, opts...)
}
