func (c *metricClient) CloseShard(
	ctx context.Context,
	request *historyservice.CloseShardRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.CloseShardResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientCloseShardScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.CloseShard(ctx, request, opts...)
}
