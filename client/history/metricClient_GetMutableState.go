func (c *metricClient) GetMutableState(
	context context.Context,
	request *historyservice.GetMutableStateRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.GetMutableStateResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientGetMutableStateScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.GetMutableState(context, request, opts...)
}
