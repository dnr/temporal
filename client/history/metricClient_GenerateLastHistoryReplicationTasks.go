func (c *metricClient) GenerateLastHistoryReplicationTasks(
	ctx context.Context,
	request *historyservice.GenerateLastHistoryReplicationTasksRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.GenerateLastHistoryReplicationTasksResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientGenerateLastHistoryReplicationTasksScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.GenerateLastHistoryReplicationTasks(ctx, request, opts...)
}
