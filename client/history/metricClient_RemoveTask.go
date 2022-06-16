func (c *metricClient) RemoveTask(
	context context.Context,
	request *historyservice.RemoveTaskRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RemoveTaskResponse, retError error) {
	resp, err := c.client.RemoveTask(context, request, opts...)

	return resp, err
}
