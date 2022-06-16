func (c *metricClient) DescribeHistoryHost(
	context context.Context,
	request *historyservice.DescribeHistoryHostRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.DescribeHistoryHostResponse, retError error) {
	resp, err := c.client.DescribeHistoryHost(context, request, opts...)

	return resp, err
}
