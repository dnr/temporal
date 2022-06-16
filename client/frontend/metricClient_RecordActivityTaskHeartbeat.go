func (c *metricClient) RecordActivityTaskHeartbeat(
	ctx context.Context,
	request *workflowservice.RecordActivityTaskHeartbeatRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RecordActivityTaskHeartbeatResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientRecordActivityTaskHeartbeatScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientRecordActivityTaskHeartbeatScope, metrics.ClientLatency)
	resp, err := c.client.RecordActivityTaskHeartbeat(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientRecordActivityTaskHeartbeatScope, metrics.ClientFailures)
	}
	return resp, err
}
