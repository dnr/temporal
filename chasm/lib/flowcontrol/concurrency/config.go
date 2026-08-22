package concurrency

import (
	"time"

	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/stream_batcher"
)

var defaultClientBatcherOptions = stream_batcher.BatcherOptions{
	MaxItems:      100,
	MinDelay:      5 * time.Millisecond,
	MaxDelay:      20 * time.Millisecond,
	IdleTime:      time.Minute,
	ClearInterval: time.Hour,
}

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

	MatchingClientBatcherOptions = dynamicconfig.NewGlobalTypedSetting(
		"flowcontrol.concurrency.matchingClientBatcher",
		defaultClientBatcherOptions,
		`Batching options for concurrency limiter requests made by matching. Fields: MaxItems, MinDelay, MaxDelay, IdleTime, and ClearInterval.`,
	)
	HistoryClientBatcherOptions = dynamicconfig.NewGlobalTypedSetting(
		"flowcontrol.concurrency.historyClientBatcher",
		defaultClientBatcherOptions,
		`Batching options for concurrency limiter releases made by the history transfer queue. Fields: MaxItems, MinDelay, MaxDelay, IdleTime, and ClearInterval.`,
	)
	ActivityClientBatcherOptions = dynamicconfig.NewGlobalTypedSetting(
		"flowcontrol.concurrency.activityClientBatcher",
		defaultClientBatcherOptions,
		`Batching options for concurrency limiter releases made by CHASM activities. Fields: MaxItems, MinDelay, MaxDelay, IdleTime, and ClearInterval.`,
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
