func (c *metricClient) GetShard(
	ctx context.Context,
	request *historyservice.GetShardRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.GetShardResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientGetShardScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.GetShard(ctx, request, opts...)
}
