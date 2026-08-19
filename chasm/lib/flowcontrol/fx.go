package flowcontrol

import (
	"go.temporal.io/server/chasm"
	"go.uber.org/fx"
)

var HistoryModule = fx.Module(
	"flowcontrol-history",
	fx.Provide(
		// TODO: newHandler,
		newLibrary,
	),
	fx.Invoke(func(l *library, registry *chasm.Registry) error {
		return registry.Register(l)
	}),
)

// TODO: add rpcs
// var FrontendModule = fx.Module(
// 	"flowcontrol-frontend",
// 	fx.Provide(newHandler),
// 	fx.Provide(newLibrary),
// 	fx.Invoke(func(l *library, registry *chasm.Registry) error {
// 		return registry.Register(l)
// 	}),
// )
