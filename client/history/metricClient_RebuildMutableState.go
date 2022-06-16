func (c *metricClient) RebuildMutableState(
	ctx context.Context,
	request *historyservice.RebuildMutableStateRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RebuildMutableStateResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRebuildMutableStateScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RebuildMutableState(ctx, request, opts...)
}
