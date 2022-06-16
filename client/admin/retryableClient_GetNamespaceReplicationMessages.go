func (c *retryableClient) GetNamespaceReplicationMessages(
	ctx context.Context,
	request *adminservice.GetNamespaceReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetNamespaceReplicationMessagesResponse, error) {
	var resp *adminservice.GetNamespaceReplicationMessagesResponse
	op := func() error {
		var err error
		resp, err = c.client.GetNamespaceReplicationMessages(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
