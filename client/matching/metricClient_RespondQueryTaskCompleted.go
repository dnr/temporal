func (c *metricClient) RespondQueryTaskCompleted(
	ctx context.Context,
	request *matchingservice.RespondQueryTaskCompletedRequest,
	opts ...grpc.CallOption,
) (_ *matchingservice.RespondQueryTaskCompletedResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.MatchingClientRespondQueryTaskCompletedScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.RespondQueryTaskCompleted(ctx, request, opts...)
}
