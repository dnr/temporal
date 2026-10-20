package fc

import (
	"time"

	"go.temporal.io/server/chasm/lib/flowcontrol/concurrency"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/stream_batcher"
)

const defaultCancelReservationTimeout = 5 * time.Second

type CleanupOptions struct {
	BatchSize        int
	ScansPerCreation int
	BusyInterval     time.Duration
	IdleInterval     time.Duration
	SweepInterval    time.Duration
}

var defaultCleanupOptions = CleanupOptions{
	BatchSize:        256,
	ScansPerCreation: 2,
	BusyInterval:     5 * time.Millisecond,
	IdleInterval:     5 * time.Second,
	SweepInterval:    time.Minute,
}

var ReadinessCleanupOptions = dynamicconfig.NewGlobalTypedSetting(
	"matching.fc.cleanupOptions",
	defaultCleanupOptions,
	`Readiness cache cleanup scheduling. BatchSize bounds candidate examinations per batch;
ScansPerCreation supplies scan credits per newly allocated limiter. BusyInterval paces batches
with outstanding credits, IdleInterval polls for new allocations, and SweepInterval supplies a
full traversal's worth of credits even without allocations. Invalid options use the defaults.`,
)

var ReadinessConcurrencyBlockedRetention = dynamicconfig.NewGlobalDurationSetting(
	"matching.fc.concurrencyBlockedRetention",
	concurrencyBlockedRetention,
	`How long to retain a concurrency limiter with zero tokens after its last transaction lookup.`,
)

var ReadinessCancelReservationTimeout = dynamicconfig.NewGlobalDurationSetting(
	"matching.fc.cancelReservationTimeout",
	defaultCancelReservationTimeout,
	`Timeout for best-effort cancellation of a concurrency reservation during rollback.`,
)

type readinessConfig struct {
	cleanupOptions           dynamicconfig.TypedPropertyFn[CleanupOptions]
	concurrencyRetention     dynamicconfig.DurationPropertyFn
	cancelReservationTimeout dynamicconfig.DurationPropertyFn
	clientBatcherOptions     dynamicconfig.TypedPropertyFn[stream_batcher.BatcherOptions]
}

func NewReadinessConfig(dc *dynamicconfig.Collection) readinessConfig {
	return readinessConfig{
		cleanupOptions:           ReadinessCleanupOptions.Get(dc),
		concurrencyRetention:     ReadinessConcurrencyBlockedRetention.Get(dc),
		cancelReservationTimeout: ReadinessCancelReservationTimeout.Get(dc),
		clientBatcherOptions:     concurrency.MatchingClientBatcherOptions.Get(dc),
	}
}

func (c readinessConfig) cleanup() CleanupOptions {
	opts := c.cleanupOptions()
	if opts.BatchSize <= 0 || opts.ScansPerCreation < 0 || opts.BusyInterval <= 0 ||
		opts.IdleInterval < opts.BusyInterval || opts.SweepInterval < opts.IdleInterval {
		return defaultCleanupOptions
	}
	return opts
}
