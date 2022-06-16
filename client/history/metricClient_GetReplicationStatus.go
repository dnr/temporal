func (c *metricClient) GetReplicationStatus(
	ctx context.Context,
	request *historyservice.GetReplicationStatusRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.GetReplicationStatusResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientGetReplicationStatusScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.GetReplicationStatus(ctx, request, opts...)
}
