package concurrency

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/stream_batcher"
)

func TestServerBatcherOptions(t *testing.T) {
	client := dynamicconfig.NewMemoryClient()
	t.Cleanup(client.OverrideSetting(ServerBatcherMaxItems, 7))
	t.Cleanup(client.OverrideSetting(ServerBatcherMinDelay, time.Millisecond))
	t.Cleanup(client.OverrideSetting(ServerBatcherMaxDelay, 2*time.Millisecond))
	t.Cleanup(client.OverrideSetting(ServerBatcherIdleTime, 3*time.Minute))
	t.Cleanup(client.OverrideSetting(ServerBatcherClearInterval, 4*time.Hour))
	collection := dynamicconfig.NewCollection(client, log.NewNoopLogger())
	collection.Start()
	t.Cleanup(collection.Stop)

	require.Equal(t, stream_batcher.BatcherOptions{
		MaxItems:      7,
		MinDelay:      time.Millisecond,
		MaxDelay:      2 * time.Millisecond,
		IdleTime:      3 * time.Minute,
		ClearInterval: 4 * time.Hour,
	}, serverBatcherOptions(collection))
}

func TestClientBatcherOptions(t *testing.T) {
	client := dynamicconfig.NewMemoryClient()
	collection := dynamicconfig.NewCollection(client, log.NewNoopLogger())
	collection.Start()
	t.Cleanup(collection.Stop)
	want := stream_batcher.BatcherOptions{
		MaxItems:      8,
		MinDelay:      2 * time.Millisecond,
		MaxDelay:      3 * time.Millisecond,
		IdleTime:      4 * time.Minute,
		ClearInterval: 5 * time.Hour,
	}
	settings := map[string]dynamicconfig.GlobalTypedSetting[stream_batcher.BatcherOptions]{
		"matching": MatchingClientBatcherOptions,
		"history":  HistoryClientBatcherOptions,
		"activity": ActivityClientBatcherOptions,
	}
	for name, setting := range settings {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(client.OverrideSetting(setting, want))
			require.Equal(t, want, setting.Get(collection)())
		})
	}
}
