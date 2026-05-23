package semaphore

import (
	"go.temporal.io/server/chasm"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
	"go.uber.org/fx"
)

// HistoryModule registers the full library (component + service handler) with
// the CHASM registry on the history service.
var HistoryModule = fx.Module(
	"chasm.lib.semaphore.history",
	fx.Provide(
		newHandler,
		newLibrary,
	),
	fx.Invoke(func(l *library, registry *chasm.Registry) error {
		return registry.Register(l)
	}),
)

// FrontendModule registers just the component on the frontend service so that
// the frontend can serialize ComponentRefs and call into the layered client.
var FrontendModule = fx.Module(
	"chasm.lib.semaphore.frontend",
	fx.Provide(semaphorepb.NewSemaphoreServiceLayeredClient),
	fx.Provide(newComponentOnlyLibrary),
	fx.Invoke(func(l *componentOnlyLibrary, registry *chasm.Registry) error {
		return registry.Register(l)
	}),
)
