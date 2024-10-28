package inline

import (
	"context"
	"errors"
	"reflect"

	"go.temporal.io/server/common/headers"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/rpc/interceptor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var (
	errHTTPGRPCStreamNotSupported = errors.New("stream not supported")
)

// inlineClientConn is a [grpc.ClientConnInterface] implementation that forwards
// requests directly to gRPC via interceptors. This implementation moves all
// outgoing metadata to incoming and takes resulting outgoing metadata and sets
// as header. But which headers to use and TLS peer context and such are
// expected to be handled by the caller.
//
// RegisterServer must not be called concurrently with itself or with Invoke,
// but after all RegisterServer calls are done (in server initialization),
// Invoke may be called concurrently.
type inlineClientConn struct {
	methods map[string]*serviceMethod
}

var _ grpc.ClientConnInterface = (*inlineClientConn)(nil)

type serviceMethod struct {
	info              grpc.UnaryServerInfo
	handler           grpc.UnaryHandler
	interceptor       grpc.UnaryServerInterceptor
	requestCounter    metrics.CounterIface
	namespaceRegistry namespace.Registry
}

var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
var protoMessageType = reflect.TypeOf((*proto.Message)(nil)).Elem()
var errorType = reflect.TypeOf((*error)(nil)).Elem()

func NewInlineClientConn() *inlineClientConn {
	return &inlineClientConn{
		methods: make(map[string]*serviceMethod),
	}
}

// RegisterServer adds a server to the inlineClientConn. This must not be called concurrently.
func (icc *inlineClientConn) RegisterServer(
	qualifiedServerName string,
	server any,
	interceptors []grpc.UnaryServerInterceptor,
	requestCounter metrics.CounterIface,
	namespaceRegistry namespace.Registry,
) {
	// Create the set of methods via reflection. We currently accept the overhead
	// of reflection compared to having to custom generate gateway code.
	serverVal := reflect.ValueOf(server)
	for i := 0; i < serverVal.Type().NumMethod(); i++ {
		reflectMethod := serverVal.Type().Method(i)
		// We intentionally look this up by name to not assume method indexes line
		// up from type to value
		methodVal := serverVal.MethodByName(reflectMethod.Name)
		// We assume the methods we want only accept a context + request and only
		// return a response + error. We also assume the method name matches the
		// RPC name.
		methodType := methodVal.Type()
		validRPCMethod := methodType.Kind() == reflect.Func &&
			methodType.NumIn() == 2 &&
			methodType.NumOut() == 2 &&
			methodType.In(0) == contextType &&
			methodType.In(1).Implements(protoMessageType) &&
			methodType.Out(0).Implements(protoMessageType) &&
			methodType.Out(1) == errorType
		if !validRPCMethod {
			continue
		}
		fullMethod := "/" + qualifiedServerName + "/" + reflectMethod.Name
		icc.methods[fullMethod] = &serviceMethod{
			info: grpc.UnaryServerInfo{Server: server, FullMethod: fullMethod},
			handler: func(ctx context.Context, req interface{}) (interface{}, error) {
				ret := methodVal.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(req)})
				err, _ := ret[1].Interface().(error)
				return ret[0].Interface(), err
			},
			interceptor:       chainUnaryServerInterceptors(interceptors),
			requestCounter:    requestCounter,
			namespaceRegistry: namespaceRegistry,
		}
	}
}

func (icc *inlineClientConn) Invoke(
	ctx context.Context,
	method string,
	args any,
	reply any,
	opts ...grpc.CallOption,
) error {
	// Move outgoing metadata to incoming and set new outgoing metadata
	md, _ := metadata.FromOutgoingContext(ctx)
	// Set the client and version headers if not already set
	if len(md[headers.ClientNameHeaderName]) == 0 {
		md.Set(headers.ClientNameHeaderName, headers.ClientNameServerHTTP)
	}
	if len(md[headers.ClientVersionHeaderName]) == 0 {
		md.Set(headers.ClientVersionHeaderName, headers.ServerVersion)
	}
	ctx = metadata.NewIncomingContext(ctx, md)
	outgoingMD := metadata.MD{}
	ctx = metadata.NewOutgoingContext(ctx, outgoingMD)

	// Get the method. Should never fail, but we check anyways
	serviceMethod := icc.methods[method]
	if serviceMethod == nil {
		return status.Error(codes.NotFound, "call not found")
	}

	// Add metric
	var namespaceTag metrics.Tag
	if namespaceName := interceptor.MustGetNamespaceName(serviceMethod.namespaceRegistry, args); namespaceName != "" {
		namespaceTag = metrics.NamespaceTag(namespaceName.String())
	} else {
		namespaceTag = metrics.NamespaceUnknownTag()
	}
	serviceMethod.requestCounter.Record(1, metrics.OperationTag(method), namespaceTag)

	// Invoke
	var resp any
	var err error
	if serviceMethod.interceptor == nil {
		resp, err = serviceMethod.handler(ctx, args)
	} else {
		resp, err = serviceMethod.interceptor(ctx, args, &serviceMethod.info, serviceMethod.handler)
	}

	// Find the header call option and set response headers. We accept that if
	// somewhere internally the metadata was replaced instead of appended to, this
	// does not work.
	for _, opt := range opts {
		if callOpt, ok := opt.(grpc.HeaderCallOption); ok {
			*callOpt.HeaderAddr = outgoingMD
		}
	}

	// Merge the response proto onto the wanted reply if non-nil
	if respProto, _ := resp.(proto.Message); respProto != nil {
		proto.Merge(reply.(proto.Message), respProto)
	}

	return err
}

func (*inlineClientConn) NewStream(
	context.Context,
	*grpc.StreamDesc,
	string,
	...grpc.CallOption,
) (grpc.ClientStream, error) {
	return nil, errHTTPGRPCStreamNotSupported
}

// Mostly taken from https://github.com/grpc/grpc-go/blob/v1.56.1/server.go#L1124-L1158
// with slight modifications.
func chainUnaryServerInterceptors(interceptors []grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	switch len(interceptors) {
	case 0:
		return nil
	case 1:
		return interceptors[0]
	default:
		return chainUnaryInterceptors(interceptors)
	}
}

func chainUnaryInterceptors(interceptors []grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return interceptors[0](ctx, req, info, getChainUnaryHandler(interceptors, 0, info, handler))
	}
}

func getChainUnaryHandler(
	interceptors []grpc.UnaryServerInterceptor,
	curr int,
	info *grpc.UnaryServerInfo,
	finalHandler grpc.UnaryHandler,
) grpc.UnaryHandler {
	if curr == len(interceptors)-1 {
		return finalHandler
	}
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		return interceptors[curr+1](ctx, req, info, getChainUnaryHandler(interceptors, curr+1, info, finalHandler))
	}
}
