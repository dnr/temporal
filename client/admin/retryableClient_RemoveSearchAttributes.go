func (c *retryableClient) RemoveSearchAttributes(
	ctx context.Context,
	request *adminservice.RemoveSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*adminservice.RemoveSearchAttributesResponse, error) {

	var resp *adminservice.RemoveSearchAttributesResponse
	op := func() error {
		var err error
		resp, err = c.client.RemoveSearchAttributes(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
