func (c *retryableClient) GetSearchAttributes(
	ctx context.Context,
	request *adminservice.GetSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetSearchAttributesResponse, error) {

	var resp *adminservice.GetSearchAttributesResponse
	op := func() error {
		var err error
		resp, err = c.client.GetSearchAttributes(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
