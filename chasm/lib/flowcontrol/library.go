package flowcontrol

import (
	"go.temporal.io/server/chasm"
)

type library struct {
	chasm.UnimplementedLibrary
}

func newLibrary() *library {
	return &library{}
}

func (l *library) Name() string {
	return "flowcontrol"
}

func (l *library) Components() []*chasm.RegistrableComponent {
	return []*chasm.RegistrableComponent{
		chasm.NewRegistrableComponent[*concurrency](
			"concurrency_limiter",
			// chasm.WithBusinessIDAlias("ConcurrencyLimiterId"), // TODO(fc): enable visibility?
		),
	}
}

// TODO: do we need any tasks?
// func (l *library) Tasks() []*chasm.RegistrableTask {
// 	return nil
// }

// TODO: add grpc handlers
// func (l *library) RegisterServices(s *grpc.Server) {
// }
