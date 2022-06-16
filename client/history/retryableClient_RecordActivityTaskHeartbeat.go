func (c *retryableClient) RecordActivityTaskHeartbeat(
	ctx context.Context,
	request *historyservice.RecordActivityTaskHeartbeatRequest,
	opts ...grpc.CallOption) (*historyservice.RecordActivityTaskHeartbeatResponse, error) {

	var resp *historyservice.RecordActivityTaskHeartbeatResponse
	op := func() error {
		var err error
		resp, err = c.client.RecordActivityTaskHeartbeat(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
