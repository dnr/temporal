func (c *metricClient) CancelOutstandingPoll(
	ctx context.Context,
	request *matchingservice.CancelOutstandingPollRequest,
	opts ...grpc.CallOption,
) (_ *matchingservice.CancelOutstandingPollResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.MatchingClientCancelOutstandingPollScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.CancelOutstandingPoll(ctx, request, opts...)
}
