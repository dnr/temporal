package dynamicconfig

import "sync"

type (
	MergedClient struct {
		clients []Client
		cancels []func()

		subscriptionLock sync.Mutex
		subscriptionIdx  int
		subscriptions    map[int]ClientUpdateFunc
	}
)

// NewMergedClient returns a client that merges configs from mulitple underlying clients.
// Clients should be ordered from higher priority to lower priority.
// Stop() should be called on the MergedClient before tearing down the server to remove
// subscriptions on the sub-clients.
func NewMergedClient(clients []Client) *MergedClient {
	return &MergedClient{
		clients:       clients,
		subscriptions: make(map[int]ClientUpdateFunc),
	}
}

func (m *MergedClient) Start() {
	// subscribe to all clients that are notifying
	for _, c := range m.clients {
		if nc, ok := c.(NotifyingClient); ok {
			cancel := nc.Subscribe(m.changed)
			m.cancels = append(m.cancels, cancel)
		}
	}
}

func (m *MergedClient) Stop() {
	for _, cancel := range m.cancels {
		cancel()
	}
}

func (m *MergedClient) changed(changed map[Key][]ConstrainedValue) {
	combinedChanges := make(map[Key][]ConstrainedValue, len(changed))
	// just re-evaluate for all changed keys. this could maybe be optimized.
	for k := range changed {
		combinedChanges[k] = m.GetValue(k)
	}
	m.subscriptionLock.Lock()
	for _, update := range m.subscriptions {
		update(combinedChanges)
	}
	m.subscriptionLock.Unlock()
}

func (m *MergedClient) GetValue(key Key) []ConstrainedValue {
	var out []ConstrainedValue
	for _, c := range m.clients {
		// this uses the fact that Collection uses the first ConstrainedValue if multiple match
		out = append(out, c.GetValue(key)...)
	}
	return out
}

func (m *MergedClient) Subscribe(update ClientUpdateFunc) (cancel func()) {
	m.subscriptionLock.Lock()
	defer m.subscriptionLock.Unlock()

	m.subscriptionIdx++
	id := m.subscriptionIdx
	m.subscriptions[id] = update

	return func() {
		m.subscriptionLock.Lock()
		defer m.subscriptionLock.Unlock()
		delete(m.subscriptions, id)
	}
}
