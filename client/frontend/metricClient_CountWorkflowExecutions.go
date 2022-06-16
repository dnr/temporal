func (c *metricClient) CountWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.CountWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.CountWorkflowExecutionsResponse, error) {

	c.metricsClient.IncCounter(metrics.FrontendClientCountWorkflowExecutionsScope, metrics.ClientRequests)

	sw := c.metricsClient.StartTimer(metrics.FrontendClientCountWorkflowExecutionsScope, metrics.ClientLatency)
	resp, err := c.client.CountWorkflowExecutions(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		c.metricsClient.IncCounter(metrics.FrontendClientCountWorkflowExecutionsScope, metrics.ClientFailures)
	}
	return resp, err
}
