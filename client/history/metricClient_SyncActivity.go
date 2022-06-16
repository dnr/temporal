func (c *metricClient) SyncActivity(
	context context.Context,
	request *historyservice.SyncActivityRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.SyncActivityResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientSyncActivityScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.SyncActivity(context, request, opts...)
}
