func (c *metricClient) TerminateWorkflowExecution(
	context context.Context,
	request *historyservice.TerminateWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.TerminateWorkflowExecutionResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientTerminateWorkflowExecutionScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.TerminateWorkflowExecution(context, request, opts...)
}
