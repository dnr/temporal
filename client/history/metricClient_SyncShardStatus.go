func (c *metricClient) SyncShardStatus(
	ctx context.Context,
	request *historyservice.SyncShardStatusRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.SyncShardStatusResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientSyncShardStatusScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.SyncShardStatus(ctx, request, opts...)
}
