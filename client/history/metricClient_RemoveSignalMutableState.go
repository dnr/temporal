func (c *metricClient) RemoveSignalMutableState(
	context context.Context,
	request *historyservice.RemoveSignalMutableStateRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RemoveSignalMutableStateResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRemoveSignalMutableStateScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RemoveSignalMutableState(context, request, opts...)
}
