package dynamicconfig_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/metrics"
)

// This file is a worked example of using MergedClient to insert a layer of static overrides
// between the dynamic config file and the defaults baked into each setting's declaration.
// The resulting precedence, highest to lowest:
//
//  1. the dynamic config file (FileBasedClient), reloaded while the server runs
//  2. static overrides fixed at startup (StaticClient), e.g. from a deployment's ServerOptions
//  3. the default in the New*Setting call in constants.go
//
// Layer 2 is what MergedClient buys you: a deployment can ship its own set of defaults
// without forking constants.go, while still letting an operator override those values at
// runtime through the config file. Note that the merge happens per constraint, not per key,
// so a namespace-constrained value in the file coexists with an unconstrained value from the
// static layer.

// fakeConfigFile is an in-memory FileReader whose contents can be replaced mid-test.
type fakeConfigFile struct {
	mu       sync.Mutex
	contents string
	modTime  time.Time
}

func newFakeConfigFile(contents string) *fakeConfigFile {
	return &fakeConfigFile{
		contents: contents,
		// Must be after the zero time, or FileBasedClient won't do its initial read.
		modTime: time.Unix(1700000000, 0),
	}
}

func (f *fakeConfigFile) write(contents string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contents = contents
	// FileBasedClient only re-reads when the modtime advances.
	f.modTime = f.modTime.Add(time.Minute)
}

func (f *fakeConfigFile) ReadFile() ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []byte(f.contents), nil
}

func (f *fakeConfigFile) GetModTime() (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.modTime, nil
}

func TestStaticOverrideLayer(t *testing.T) {
	dynamicconfig.ResetRegistryForTest()

	// Layer 3: settings with defaults baked in, as declared in constants.go.
	var (
		maxCallsPerSecond = dynamicconfig.NewGlobalIntSetting(
			"demo.maxCallsPerSecond", 100, `Demo setting overridden in both layers.`)
		enableFeature = dynamicconfig.NewNamespaceBoolSetting(
			"demo.enableFeature", false, `Demo setting overridden in the static layer only.`)
		cacheTTL = dynamicconfig.NewGlobalDurationSetting(
			"demo.cacheTTL", time.Minute, `Demo setting not overridden anywhere.`)
	)

	// Layer 2: static overrides, fixed for the lifetime of the process. In real code these
	// would come from wherever the deployment configures itself; here we just write them out.
	staticOverrides := dynamicconfig.StaticClient{
		maxCallsPerSecond.Key(): 500,
		enableFeature.Key():     true,
	}

	// Layer 1: the operator's dynamic config file. It raises maxCallsPerSecond above the
	// static override, and turns enableFeature back off for one namespace only.
	file := newFakeConfigFile(`
demo.maxCallsPerSecond:
- value: 900
  constraints: {}

demo.enableFeature:
- value: false
  constraints:
    namespace: locked-down-namespace
`)

	logger := log.NewNoopLogger()
	doneCh := make(chan any)
	defer close(doneCh)

	fileClient, err := dynamicconfig.NewFileBasedClientWithReader(file,
		&dynamicconfig.FileBasedClientConfig{
			Filepath:     "demo-config.yaml",
			PollInterval: time.Minute, // long enough that we control reloads ourselves
		}, logger, doneCh, metrics.NoopMetricsHandler)
	require.NoError(t, err)

	// Clients are ordered highest priority first.
	client := dynamicconfig.NewMergedClient([]dynamicconfig.Client{fileClient, staticOverrides})
	client.Start()
	defer client.Stop()

	collection := dynamicconfig.NewCollection(client, logger)
	collection.Start()
	defer collection.Stop()

	// Set in neither layer: the baked-in default wins.
	assert.Equal(t, time.Minute, cacheTTL.Get(collection)())

	// Set in both layers: the file wins, because it's the higher-priority client and
	// Collection takes the first matching ConstrainedValue.
	assert.Equal(t, 900, maxCallsPerSecond.Get(collection)())

	// Set in the static layer only: it wins over the baked-in default of false.
	assert.True(t, enableFeature.Get(collection)("some-namespace"))

	// ...except where the file has a more specific constraint. The static layer's
	// unconstrained value and the file's namespace-constrained value both survive the merge,
	// and normal namespace precedence picks between them.
	assert.False(t, enableFeature.Get(collection)("locked-down-namespace"))

	// Subscriptions see through the merge too. Note that the initial value is returned
	// synchronously, while later updates arrive on the callback.
	updates := make(chan int, 1)
	initial, cancel := maxCallsPerSecond.Subscribe(collection)(func(v int) { updates <- v })
	defer cancel()
	assert.Equal(t, 900, initial)

	// The operator removes their override from the file. The value falls back to the static
	// layer (500), not all the way to the baked-in default (100).
	file.write(`
demo.enableFeature:
- value: false
  constraints:
    namespace: locked-down-namespace
`)
	require.NoError(t, fileClient.Update())

	select {
	case v := <-updates:
		assert.Equal(t, 500, v)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for subscription callback")
	}
	assert.Equal(t, 500, maxCallsPerSecond.Get(collection)())
}
