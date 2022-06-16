func (c *metricClient) PollWorkflowTaskQueue(
	ctx context.Context,
	request *matchingservice.PollWorkflowTaskQueueRequest,
	opts ...grpc.CallOption,
) (_ *matchingservice.PollWorkflowTaskQueueResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.MatchingClientPollWorkflowTaskQueueScope)
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

	return c.client.PollWorkflowTaskQueue(ctx, request, opts...)
}
