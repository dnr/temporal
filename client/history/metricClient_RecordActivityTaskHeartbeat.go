func (c *metricClient) RecordActivityTaskHeartbeat(
	ctx context.Context,
	request *historyservice.RecordActivityTaskHeartbeatRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RecordActivityTaskHeartbeatResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRecordActivityTaskHeartbeatScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RecordActivityTaskHeartbeat(ctx, request, opts...)
}
