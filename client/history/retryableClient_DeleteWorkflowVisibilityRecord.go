func (c *retryableClient) DeleteWorkflowVisibilityRecord(
	ctx context.Context,
	request *historyservice.DeleteWorkflowVisibilityRecordRequest,
	opts ...grpc.CallOption,
) (*historyservice.DeleteWorkflowVisibilityRecordResponse, error) {

	var resp *historyservice.DeleteWorkflowVisibilityRecordResponse
	op := func() error {
		var err error
		resp, err = c.client.DeleteWorkflowVisibilityRecord(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
