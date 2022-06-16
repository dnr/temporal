func (c *retryableClient) MergeDLQMessages(
	ctx context.Context,
	request *adminservice.MergeDLQMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.MergeDLQMessagesResponse, error) {

	var resp *adminservice.MergeDLQMessagesResponse
	op := func() error {
		var err error
		resp, err = c.client.MergeDLQMessages(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
