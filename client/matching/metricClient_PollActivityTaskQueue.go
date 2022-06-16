func (c *metricClient) PollActivityTaskQueue(
	ctx context.Context,
	request *matchingservice.PollActivityTaskQueueRequest,
	opts ...grpc.CallOption,
) (_ *matchingservice.PollActivityTaskQueueResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.MatchingClientPollActivityTaskQueueScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	if request.PollRequest != nil {
		c.emitForwardedSourceStats(
			scope,
			request.GetForwardedSource(),
			request.PollRequest.TaskQueue,
		)
	}

	return c.client.PollActivityTaskQueue(ctx, request, opts...)
}
