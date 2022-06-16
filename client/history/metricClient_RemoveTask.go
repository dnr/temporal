func (c *metricClient) RemoveTask(
	ctx context.Context,
	request *historyservice.RemoveTaskRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RemoveTaskResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRemoveTaskScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RemoveTask(ctx, request, opts...)
}
