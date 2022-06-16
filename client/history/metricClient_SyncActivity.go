func (c *metricClient) SyncActivity(
	ctx context.Context,
	request *historyservice.SyncActivityRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.SyncActivityResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientSyncActivityScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.SyncActivity(ctx, request, opts...)
}
