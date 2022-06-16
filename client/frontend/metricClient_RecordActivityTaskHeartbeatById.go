func (c *metricClient) RecordActivityTaskHeartbeatById(
	ctx context.Context,
	request *workflowservice.RecordActivityTaskHeartbeatByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RecordActivityTaskHeartbeatByIdResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientRecordActivityTaskHeartbeatByIdScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientRecordActivityTaskHeartbeatByIdScope, metrics.ClientLatency)
	resp, err := c.client.RecordActivityTaskHeartbeatById(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientRecordActivityTaskHeartbeatByIdScope, metrics.ClientFailures)
	}
	return resp, err
}
