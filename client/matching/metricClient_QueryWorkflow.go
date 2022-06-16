func (c *metricClient) QueryWorkflow(
	ctx context.Context,
	request *matchingservice.QueryWorkflowRequest,
	opts ...grpc.CallOption,
) (_ *matchingservice.QueryWorkflowResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.MatchingClientQueryWorkflowScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	c.emitForwardedSourceStats(
		scope,
		request.GetForwardedSource(),
		request.TaskQueue,
	)

	return c.client.QueryWorkflow(ctx, request, opts...)
}
