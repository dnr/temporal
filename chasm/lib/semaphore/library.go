package semaphore

import (
	"go.temporal.io/server/chasm"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
	"google.golang.org/grpc"
)

const (
	LibraryName   = "semaphore"
	ComponentName = "semaphore"
)

var (
	Archetype   = chasm.FullyQualifiedName(LibraryName, ComponentName)
	ArchetypeID = chasm.GenerateTypeID(Archetype)
)

// componentOnlyLibrary registers just the component, with no task or service
// handlers. Used by the frontend service so it can serialize ComponentRefs.
type componentOnlyLibrary struct {
	chasm.UnimplementedLibrary
}

func newComponentOnlyLibrary() *componentOnlyLibrary {
	return &componentOnlyLibrary{}
}

func (l *componentOnlyLibrary) Name() string {
	return LibraryName
}

func (l *componentOnlyLibrary) Components() []*chasm.RegistrableComponent {
	return []*chasm.RegistrableComponent{
		chasm.NewRegistrableComponent[*Semaphore](
			ComponentName,
			chasm.WithBusinessIDAlias("SemaphoreId"),
		),
	}
}

// library is the full library used by the service that owns the engine
// (history): it includes the gRPC handler.
type library struct {
	componentOnlyLibrary

	handler *handler
}

func newLibrary(handler *handler) *library {
	return &library{
		componentOnlyLibrary: *newComponentOnlyLibrary(),
		handler:              handler,
	}
}

func (l *library) RegisterServices(server *grpc.Server) {
	server.RegisterService(&semaphorepb.SemaphoreService_ServiceDesc, l.handler)
}
