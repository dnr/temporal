func (c *metricClient) RequestCancelWorkflowExecution(
	context context.Context,
	request *historyservice.RequestCancelWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RequestCancelWorkflowExecutionResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientRequestCancelWorkflowExecutionScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RequestCancelWorkflowExecution(context, request, opts...)
}
