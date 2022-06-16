func (c *metricClient) ListTaskQueuePartitions(
	ctx context.Context,
	request *matchingservice.ListTaskQueuePartitionsRequest,
	opts ...grpc.CallOption,
) (_ *matchingservice.ListTaskQueuePartitionsResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.MatchingClientListTaskQueuePartitionsScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.ListTaskQueuePartitions(ctx, request, opts...)
}
