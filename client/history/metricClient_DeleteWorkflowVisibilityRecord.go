func (c *metricClient) DeleteWorkflowVisibilityRecord(
	ctx context.Context,
	request *historyservice.DeleteWorkflowVisibilityRecordRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.DeleteWorkflowVisibilityRecordResponse, retError error) {

	scope, stopwatch := c.startMetricsRecording(metrics.HistoryClientDeleteWorkflowVisibilityRecordScope)
	defer func() {
		c.finishMetricsRecording(scope, stopwatch, retError)
	}()

	return c.client.DeleteWorkflowVisibilityRecord(ctx, request, opts...)
}
