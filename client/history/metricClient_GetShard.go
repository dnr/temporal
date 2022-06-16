func (c *metricClient) GetShard(
	context context.Context,
	request *historyservice.GetShardRequest,
	opts ...grpc.CallOption,
) (_ *historyservice.GetShardResponse, retError error) {
	resp, err := c.client.GetShard(context, request, opts...)

	return resp, err
}
