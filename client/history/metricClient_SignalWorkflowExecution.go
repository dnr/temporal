func (c *metricClient) SignalWorkflowExecution(
	context context.Context,
	request *historyservice.SignalWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.SignalWorkflowExecutionResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientSignalWorkflowExecutionScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.SignalWorkflowExecution(context, request, opts...)
}
