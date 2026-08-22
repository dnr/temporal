package concurrency

import (
	"time"

	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/stream_batcher"
)

var (
	ServerBatcherMaxItems = dynamicconfig.NewGlobalIntSetting(
		"flowcontrol.concurrency.serverBatcher.maxItems",
		100,
		`Maximum number of concurrency limiter requests combined in one server-side batch.`,
	)
	ServerBatcherMinDelay = dynamicconfig.NewGlobalDurationSetting(
		"flowcontrol.concurrency.serverBatcher.minDelay",
		5*time.Millisecond,
		`Minimum quiet period before a server-side concurrency limiter batch is processed.`,
	)
	ServerBatcherMaxDelay = dynamicconfig.NewGlobalDurationSetting(
		"flowcontrol.concurrency.serverBatcher.maxDelay",
		10*time.Millisecond,
		`Maximum time the first request waits for a server-side concurrency limiter batch to fill.`,
	)
	ServerBatcherIdleTime = dynamicconfig.NewGlobalDurationSetting(
		"flowcontrol.concurrency.serverBatcher.idleTime",
		time.Minute,
		`How long an idle server-side concurrency limiter batcher goroutine is retained.`,
	)
	ServerBatcherClearInterval = dynamicconfig.NewGlobalDurationSetting(
		"flowcontrol.concurrency.serverBatcher.clearInterval",
		time.Hour,
		`How often cached server-side concurrency limiter batchers are cleared.`,
	)
)

func serverBatcherOptions(dc *dynamicconfig.Collection) stream_batcher.BatcherOptions {
	return stream_batcher.BatcherOptions{
		MaxItems:      ServerBatcherMaxItems.Get(dc)(),
		MinDelay:      ServerBatcherMinDelay.Get(dc)(),
		MaxDelay:      ServerBatcherMaxDelay.Get(dc)(),
		IdleTime:      ServerBatcherIdleTime.Get(dc)(),
		ClearInterval: ServerBatcherClearInterval.Get(dc)(),
	}
}
