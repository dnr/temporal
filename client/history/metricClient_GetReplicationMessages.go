func (c *metricClient) GetReplicationMessages(
	context context.Context,
	request *historyservice.GetReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.GetReplicationMessagesResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientGetReplicationTasksScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.GetReplicationMessages(context, request, opts...)
}
