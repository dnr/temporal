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
	want := stream_batcher.BatcherOptions{
		MaxItems:      7,
		MinDelay:      time.Millisecond,
		MaxDelay:      2 * time.Millisecond,
		IdleTime:      3 * time.Minute,
		ClearInterval: 4 * time.Hour,
	}
	t.Cleanup(client.OverrideSetting(ServerBatcherOptions, want))
	collection := dynamicconfig.NewCollection(client, log.NewNoopLogger())
	collection.Start()
	t.Cleanup(collection.Stop)

	require.Equal(t, want, serverBatcherOptions(collection))
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
