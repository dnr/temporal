func (c *metricClient) AddWorkflowTask(
	ctx context.Context,
	request *matchingservice.AddWorkflowTaskRequest,
	opts ...grpc.CallOption,
) (_ *matchingservice.AddWorkflowTaskResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.MatchingClientAddWorkflowTaskScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	c.emitForwardedSourceStats(
		scope,
		request.GetForwardedSource(),
		request.TaskQueue,
	)

	return c.client.AddWorkflowTask(ctx, request, opts...)
}
