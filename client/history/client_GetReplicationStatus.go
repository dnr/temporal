func (c *clientImpl) GetReplicationStatus(
	ctx context.Context,
	request *historyservice.GetReplicationStatusRequest,
	opts ...grpc.CallOption,
) (*historyservice.GetReplicationStatusResponse, error) {
	clients, err := c.clients.GetAllClients()
	if err != nil {
		return nil, err
	}

	respChan := make(chan *historyservice.GetReplicationStatusResponse, len(clients))
	errChan := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(len(clients))
	for _, client := range clients {
		historyClient := client.(historyservice.HistoryServiceClient)
		go func(client historyservice.HistoryServiceClient) {
			defer wg.Done()
			resp, err := historyClient.GetReplicationStatus(ctx, request, opts...)
			if err != nil {
				select {
				case errChan <- err:
				default:
				}
			} else {
				respChan <- resp
			}
		}(historyClient)
	}
	wg.Wait()
	close(respChan)
	close(errChan)

	response := &historyservice.GetReplicationStatusResponse{}
	for resp := range respChan {
		response.Shards = append(response.Shards, resp.Shards...)
	}

	if len(errChan) > 0 {
		err = <-errChan
		return response, err
	}

	return response, nil
}
