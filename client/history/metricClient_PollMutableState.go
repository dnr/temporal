func (c *metricClient) PollMutableState(
	ctx context.Context,
	request *historyservice.PollMutableStateRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.PollMutableStateResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientPollMutableStateScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.PollMutableState(ctx, request, opts...)
}
