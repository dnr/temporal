func (c *retryableClient) ReplicateEventsV2(
	ctx context.Context,
	request *historyservice.ReplicateEventsV2Request,
	opts ...grpc.CallOption) (*historyservice.ReplicateEventsV2Response, error) {

	var resp *historyservice.ReplicateEventsV2Response
	op := func() error {
		var err error
		resp, err = c.client.ReplicateEventsV2(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
