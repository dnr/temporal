func (c *metricClient) ReplicateEventsV2(
	ctx context.Context,
	request *historyservice.ReplicateEventsV2Request,
	opts ...grpc.CallOption,
) (_ *historyservice.ReplicateEventsV2Response, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientReplicateEventsV2Scope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.ReplicateEventsV2(ctx, request, opts...)
}
