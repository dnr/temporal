func (c *retryableClient) ReapplyEvents(
	ctx context.Context,
	request *adminservice.ReapplyEventsRequest,
	opts ...grpc.CallOption,
) (*adminservice.ReapplyEventsResponse, error) {
	var resp *adminservice.ReapplyEventsResponse
	op := func() error {
		var err error
		resp, err = c.client.ReapplyEvents(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
