func (c *metricClient) DescribeMutableState(
	context context.Context,
	request *historyservice.DescribeMutableStateRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.DescribeMutableStateResponse, retError error) {
	resp, err := c.client.DescribeMutableState(context, request, opts...)

	return resp, err
}
