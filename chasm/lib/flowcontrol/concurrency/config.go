package concurrency

import (
	"time"

	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/stream_batcher"
)

var (
	defaultServerBatcherOptions = stream_batcher.BatcherOptions{
		MaxItems:      100,
		MinDelay:      5 * time.Millisecond,
		MaxDelay:      10 * time.Millisecond,
		IdleTime:      time.Minute,
		ClearInterval: time.Hour,
	}
	defaultClientBatcherOptions = stream_batcher.BatcherOptions{
		MaxItems:      100,
		MinDelay:      5 * time.Millisecond,
		MaxDelay:      20 * time.Millisecond,
		IdleTime:      time.Minute,
		ClearInterval: time.Hour,
	}
)

var (
	ServerBatcherOptions = dynamicconfig.NewGlobalTypedSetting(
		"flowcontrol.concurrency.serverBatcher",
		defaultServerBatcherOptions,
		`Batching options for concurrency limiter requests received by the server. Fields: MaxItems, MinDelay, MaxDelay, IdleTime, and ClearInterval.`,
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
	return ServerBatcherOptions.Get(dc)()
}
