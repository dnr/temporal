func (c *retryableClient) AddSearchAttributes(
	ctx context.Context,
	request *adminservice.AddSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*adminservice.AddSearchAttributesResponse, error) {

	var resp *adminservice.AddSearchAttributesResponse
	op := func() error {
		var err error
		resp, err = c.client.AddSearchAttributes(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
