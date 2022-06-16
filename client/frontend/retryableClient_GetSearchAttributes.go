func (c *retryableClient) GetSearchAttributes(
	ctx context.Context,
	request *workflowservice.GetSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetSearchAttributesResponse, error) {
	var resp *workflowservice.GetSearchAttributesResponse
	op := func() error {
		var err error
		resp, err = c.client.GetSearchAttributes(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
