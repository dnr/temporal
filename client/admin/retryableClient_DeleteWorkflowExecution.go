func (c *retryableClient) DeleteWorkflowExecution(
	ctx context.Context,
	request *adminservice.DeleteWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*adminservice.DeleteWorkflowExecutionResponse, error) {

	var resp *adminservice.DeleteWorkflowExecutionResponse
	op := func() error {
		var err error
		resp, err = c.client.DeleteWorkflowExecution(ctx, request, opts...)
		return err
	}
	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
