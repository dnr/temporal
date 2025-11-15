package dynamicconfig

import (
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/pingable"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Provide(func(client Client, logger log.Logger, lc fx.Lifecycle) *Collection {
		col := NewCollection(client, logger)
		lc.Append(fx.StartStopHook(col.Start, col.Stop))
		return col
	}),
	fx.Provide(fx.Annotate(
		func(c *Collection) pingable.Pingable { return c },
		fx.ResultTags(`group:"deadlockDetectorRoots"`),
	)),
	fx.Invoke(func(client Client, registry namespace.Registry) {
		// Client is constructed outside of fx or by ServerOptionsProvider and supplied to the
		// individual service fx apps, so it can't depend on namespace.Registry. We have to
		// hook it up after init.
		if mc, ok := client.(NamespaceMappingClient); ok {
			mc.SetNamespaceRegistry(registry)
		}
	}),
)
