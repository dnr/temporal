func (c *metricClient) GetWorkflowExecutionRawHistoryV2(
	ctx context.Context,
	request *adminservice.GetWorkflowExecutionRawHistoryV2Request,
	opts ...grpc.CallOption,
) (*adminservice.GetWorkflowExecutionRawHistoryV2Response, error) {

	c.metricsClient.IncCounter(metrics.AdminClientGetWorkflowExecutionRawHistoryV2Scope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.AdminClientGetWorkflowExecutionRawHistoryV2Scope, metrics.ClientLatency)
	resp, err := c.client.GetWorkflowExecutionRawHistoryV2(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.AdminClientGetWorkflowExecutionRawHistoryV2Scope, metrics.ClientFailures)
	}
	return resp, err
}
