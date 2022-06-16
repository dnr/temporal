func (c *metricClient) DescribeTaskQueue(
	ctx context.Context,
	request *matchingservice.DescribeTaskQueueRequest,
	opts ...grpc.CallOption,
) (_ *matchingservice.DescribeTaskQueueResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.MatchingClientDescribeTaskQueueScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.DescribeTaskQueue(ctx, request, opts...)
}
