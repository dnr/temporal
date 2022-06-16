func (c *retryableClient) RecordActivityTaskHeartbeatById(
	ctx context.Context,
	request *workflowservice.RecordActivityTaskHeartbeatByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RecordActivityTaskHeartbeatByIdResponse, error) {
	var resp *workflowservice.RecordActivityTaskHeartbeatByIdResponse
	op := func() error {
		var err error
		resp, err = c.client.RecordActivityTaskHeartbeatById(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
