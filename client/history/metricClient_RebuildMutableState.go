func (c *metricClient) RebuildMutableState(
	context context.Context,
	request *historyservice.RebuildMutableStateRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.RebuildMutableStateResponse, retError error) {
	resp, err := c.client.RebuildMutableState(context, request, opts...)

	return resp, err
}
