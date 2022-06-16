func (c *retryableClient) GetDLQMessages(
	ctx context.Context,
	request *historyservice.GetDLQMessagesRequest,
	opts ...grpc.CallOption,
) (*historyservice.GetDLQMessagesResponse, error) {

	var resp *historyservice.GetDLQMessagesResponse
	op := func() error {
		var err error
		resp, err = c.client.GetDLQMessages(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
