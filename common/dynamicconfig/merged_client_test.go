package dynamicconfig_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/dynamicconfig"
)

// stableClient is a NotifyingClient that honors the Client.GetValue contract of returning
// the same slice as long as the value hasn't changed.
type stableClient struct {
	dynamicconfig.NotifyingClientImpl
	mu     sync.Mutex
	values map[dynamicconfig.Key][]dynamicconfig.ConstrainedValue
}

func newStableClient() *stableClient {
	return &stableClient{
		NotifyingClientImpl: dynamicconfig.NewNotifyingClientImpl(),
		values:              make(map[dynamicconfig.Key][]dynamicconfig.ConstrainedValue),
	}
}

func (c *stableClient) GetValue(key dynamicconfig.Key) []dynamicconfig.ConstrainedValue {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.values[key]
}

func (c *stableClient) set(key dynamicconfig.Key, values ...any) {
	var cvs []dynamicconfig.ConstrainedValue
	for _, v := range values {
		cvs = append(cvs, dynamicconfig.ConstrainedValue{Value: v})
	}
	c.mu.Lock()
	c.values[key] = cvs
	c.mu.Unlock()
	c.PublishUpdates(map[dynamicconfig.Key][]dynamicconfig.ConstrainedValue{key: cvs})
}

func sameSlice(a, b []dynamicconfig.ConstrainedValue) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

func TestMergedClient(t *testing.T) {
	c1 := newStableClient()
	c2 := newStableClient()
	m := dynamicconfig.NewMergedClient([]dynamicconfig.Client{c1, c2})
	m.Start()
	defer m.Stop()

	k := dynamicconfig.MakeKey("test-key")
	other := dynamicconfig.MakeKey("other-key")

	assert.Nil(t, m.GetValue(k))

	c1.set(k, 1)
	c2.set(k, 2)

	// higher priority client's values come first
	v := m.GetValue(k)
	require.Len(t, v, 2)
	assert.Equal(t, 1, v[0].Value)
	assert.Equal(t, 2, v[1].Value)

	// while nothing changes, GetValue must return the identical slice, since callers cache
	// converted values keyed by weak pointers into it
	assert.True(t, sameSlice(v, m.GetValue(k)), "expected identical slice for unchanged value")

	// notifications carry the same slice that later GetValue calls return
	var notified map[dynamicconfig.Key][]dynamicconfig.ConstrainedValue
	cancel := m.Subscribe(func(changed map[dynamicconfig.Key][]dynamicconfig.ConstrainedValue) {
		notified = changed
	})
	defer cancel()

	c2.set(k, 3)
	require.Contains(t, notified, k)
	nv := notified[k]
	require.Len(t, nv, 2)
	assert.Equal(t, 1, nv[0].Value)
	assert.Equal(t, 3, nv[1].Value)
	assert.True(t, sameSlice(nv, m.GetValue(k)), "expected GetValue to return the notified slice")

	// changes to other keys don't invalidate this key's slice
	c1.set(other, "hello")
	assert.True(t, sameSlice(nv, m.GetValue(k)), "expected identical slice after unrelated change")

	// removing the value from one client drops it from the merge
	c1.set(k)
	v = m.GetValue(k)
	require.Len(t, v, 1)
	assert.Equal(t, 3, v[0].Value)

	// removing it everywhere yields nil (as a deletion notification)
	c2.set(k)
	require.Contains(t, notified, k)
	assert.Nil(t, notified[k])
	assert.Nil(t, m.GetValue(k))
}
