package flowcontrol

import (
	"go.temporal.io/server/chasm"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.uber.org/fx"
)

var HistoryModule = fx.Module(
	"flowcontrol-history",
	fx.Provide(
		newConcurrencyHandler,
		newLibrary,
		fcpb.NewConcurrencyServiceLayeredClient,
	),
	fx.Invoke(func(l *library, registry *chasm.Registry) error {
		return registry.Register(l)
	}),
)

var MatchingModule = fx.Module(
	"flowcontrol-matching",
	fx.Provide(
		fcpb.NewConcurrencyServiceLayeredClient,
	),
)
