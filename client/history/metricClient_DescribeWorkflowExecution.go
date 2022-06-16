func (c *metricClient) DescribeWorkflowExecution(
	context context.Context,
	request *historyservice.DescribeWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.DescribeWorkflowExecutionResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientDescribeWorkflowExecutionScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.DescribeWorkflowExecution(context, request, opts...)
}
