func (c *metricClient) VerifyFirstWorkflowTaskScheduled(
	ctx context.Context,
	request *historyservice.VerifyFirstWorkflowTaskScheduledRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.VerifyFirstWorkflowTaskScheduledResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientVerifyFirstWorkflowTaskScheduledScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.VerifyFirstWorkflowTaskScheduled(ctx, request, opts...)
}
