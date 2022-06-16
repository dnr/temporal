func (c *metricClient) StartWorkflowExecution(
	context context.Context,
	request *historyservice.StartWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.StartWorkflowExecutionResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientStartWorkflowExecutionScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.StartWorkflowExecution(context, request, opts...)
}
