package semaphore

import (
	"go.temporal.io/server/chasm"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
	"go.uber.org/fx"
)

// HistoryModule registers the full library (component + service handler +
// task handlers) on the history service, which is where the CHASM engine
// runs.
var HistoryModule = fx.Module(
	"chasm.lib.semaphore.history",
	fx.Provide(
		newHandler,
		newReservationExpiryTaskHandler,
		newLibrary,
	),
	fx.Invoke(func(l *library, registry *chasm.Registry) error {
		return registry.Register(l)
	}),
)

// ClientModule registers just the component (no task or service handlers),
// and exposes the SemaphoreService layered client. Wire this into services
// that need to call the SemaphoreService — currently matching (for
// Reserve/Commit) and history (for Release).
//
// Intentionally NOT wired into the frontend service: the semaphore API is
// internal-only for now.
var ClientModule = fx.Module(
	"chasm.lib.semaphore.client",
	fx.Provide(semaphorepb.NewSemaphoreServiceLayeredClient),
	fx.Provide(newComponentOnlyLibrary),
	fx.Invoke(func(l *componentOnlyLibrary, registry *chasm.Registry) error {
		return registry.Register(l)
	}),
)
