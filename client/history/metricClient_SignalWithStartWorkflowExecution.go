func (c *metricClient) SignalWithStartWorkflowExecution(
	context context.Context,
	request *historyservice.SignalWithStartWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.SignalWithStartWorkflowExecutionResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientSignalWithStartWorkflowExecutionScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.SignalWithStartWorkflowExecution(context, request, opts...)
}
