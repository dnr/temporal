func (c *retryableClient) QueryWorkflow(
	ctx context.Context,
	queryRequest *matchingservice.QueryWorkflowRequest,
	opts ...grpc.CallOption) (*matchingservice.QueryWorkflowResponse, error) {

	var resp *matchingservice.QueryWorkflowResponse
	op := func() error {
		var err error
		resp, err = c.client.QueryWorkflow(ctx, queryRequest, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
