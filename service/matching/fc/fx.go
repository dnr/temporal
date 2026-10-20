package fc

import (
	"go.temporal.io/server/chasm/lib/flowcontrol"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/dynamicconfig"
	"go.uber.org/fx"
)

var Module = fx.Options(
	flowcontrol.MatchingModule,
	fx.Provide(readinessProvider),
	fx.Invoke(func(lc fx.Lifecycle, readiness *Readiness) {
		lc.Append(fx.StartStopHook(readiness.Start, readiness.Stop))
	}),
)

func readinessProvider(
	timeSource clock.TimeSource,
	concurrencyServiceClient fcpb.ConcurrencyServiceClient,
	dc *dynamicconfig.Collection,
) *Readiness {
	return NewReadiness(timeSource, concurrencyServiceClient, NewReadinessConfig(dc))
}
