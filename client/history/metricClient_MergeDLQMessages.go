func (c *metricClient) MergeDLQMessages(
	ctx context.Context,
	request *historyservice.MergeDLQMessagesRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.MergeDLQMessagesResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientMergeDLQMessagesScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.MergeDLQMessages(ctx, request, opts...)
}
