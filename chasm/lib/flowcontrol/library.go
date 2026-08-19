package flowcontrol

import (
	"go.temporal.io/server/chasm"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"google.golang.org/grpc"
)

type library struct {
	chasm.UnimplementedLibrary

	concurrencyHandler *concurrencyHandler
}

func newLibrary(handler *concurrencyHandler) *library {
	return &library{
		concurrencyHandler: handler,
	}
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

// TODO(fc): add tasks for notification backoff
// func (l *library) Tasks() []*chasm.RegistrableTask {
// 	return nil
// }

func (l *library) RegisterServices(s *grpc.Server) {
	s.RegisterService(&fcpb.ConcurrencyService_ServiceDesc, l.concurrencyHandler)
}
