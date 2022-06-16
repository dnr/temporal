func (c *metricClient) QueryWorkflow(
	context context.Context,
	request *historyservice.QueryWorkflowRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.QueryWorkflowResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientQueryWorkflowScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.QueryWorkflow(context, request, opts...)
}
