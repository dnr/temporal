func (c *metricClient) GetDLQMessages(
	ctx context.Context,
	request *historyservice.GetDLQMessagesRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.GetDLQMessagesResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientGetDLQMessagesScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.GetDLQMessages(ctx, request, opts...)
}
