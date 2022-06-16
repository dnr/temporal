func (c *metricClient) DescribeHistoryHost(
	ctx context.Context,
	request *historyservice.DescribeHistoryHostRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.DescribeHistoryHostResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientDescribeHistoryHostScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.DescribeHistoryHost(ctx, request, opts...)
}
