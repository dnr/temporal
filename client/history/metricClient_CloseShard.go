func (c *metricClient) CloseShard(
	context context.Context,
	request *historyservice.CloseShardRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.CloseShardResponse, retError error) {
	resp, err := c.client.CloseShard(context, request, opts...)

	return resp, err
}
