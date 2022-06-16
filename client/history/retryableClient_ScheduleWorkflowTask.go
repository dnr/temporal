func (c *retryableClient) ScheduleWorkflowTask(
	ctx context.Context,
	request *historyservice.ScheduleWorkflowTaskRequest,
	opts ...grpc.CallOption) (*historyservice.ScheduleWorkflowTaskResponse, error) {

	var resp *historyservice.ScheduleWorkflowTaskResponse
	op := func() error {
		var err error
		resp, err = c.client.ScheduleWorkflowTask(ctx, request, opts...)
		return err
	}

	err := backoff.Retry(op, c.policy, c.isRetryable)
	return resp, err
}
