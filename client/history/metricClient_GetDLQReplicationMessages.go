func (c *metricClient) GetDLQReplicationMessages(
	context context.Context,
	request *historyservice.GetDLQReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.GetDLQReplicationMessagesResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientGetDLQReplicationTasksScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.GetDLQReplicationMessages(context, request, opts...)
}
