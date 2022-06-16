func (c *metricClient) AddActivityTask(
	ctx context.Context,
	request *matchingservice.AddActivityTaskRequest,
	opts ...grpc.CallOption,
) (_ *matchingservice.AddActivityTaskResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.MatchingClientAddActivityTaskScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	c.emitForwardedSourceStats(
		scope,
		request.GetForwardedSource(),
		request.TaskQueue,
	)

	return c.client.AddActivityTask(ctx, request, opts...)
}
