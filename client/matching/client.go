// Generates all three generated files in this package:
//go:generate go run ../../cmd/tools/genrpcwrappers -service matching

package matching

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/server/api/matchingservice/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/debug"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/metrics"
	serviceerrors "go.temporal.io/server/common/serviceerror"
	"go.temporal.io/server/common/tqid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var _ matchingservice.MatchingServiceClient = (*clientImpl)(nil)

const (
	// DefaultTimeout is the max timeout for regular calls
	DefaultTimeout = time.Minute * debug.TimeoutMultiplier
	// DefaultLongPollTimeout is the max timeout for long poll calls
	DefaultLongPollTimeout = time.Minute * 5 * debug.TimeoutMultiplier
)

type clientImpl struct {
	timeout         time.Duration
	longPollTimeout time.Duration
	clients         common.ClientCache
	metricsHandler  metrics.Handler
	logger          log.Logger
	loadBalancer    LoadBalancer
	spreadRouting   dynamicconfig.TypedPropertyFn[dynamicconfig.GradualChange[int]]
	partitionCache  *partitionCache
}

// NewClient creates a new history service gRPC client
func NewClient(
	timeout time.Duration,
	longPollTimeout time.Duration,
	clients common.ClientCache,
	metricsHandler metrics.Handler,
	logger log.Logger,
	lb LoadBalancer,
	spreadRouting dynamicconfig.TypedPropertyFn[dynamicconfig.GradualChange[int]],
) matchingservice.MatchingServiceClient {
	c := &clientImpl{
		timeout:         timeout,
		longPollTimeout: longPollTimeout,
		clients:         clients,
		metricsHandler:  metricsHandler,
		logger:          logger,
		loadBalancer:    lb,
		spreadRouting:   spreadRouting,
		partitionCache:  newPartitionCache(),
	}

	// Start goroutine to prune partition count cache.
	// Clean up on gc, since we can't easily hook into fx here.
	c.partitionCache.Start()
	runtime.AddCleanup(c, func(cache *partitionCache) { cache.Stop() }, c.partitionCache)

	return c
}

func handlePartitionCounts[Req, Res any](
	ctx context.Context,
	c *clientImpl,
	pkey string,
	kind enumspb.TaskQueueKind,
	request Req,
	opts []grpc.CallOption,
	op func(
		ctx context.Context,
		pc PartitionCounts,
		request Req,
		opts []grpc.CallOption,
	) (Res, error),
) (Res, error) {
	if kind != enumspb.TASK_QUEUE_KIND_NORMAL {
		// only normal partitions participate in scaling
		return op(ctx, PartitionCounts{}, request, opts)
	}

	// capture trailer
	var trailer metadata.MD
	opts = append(slices.Clone(opts), grpc.Trailer(&trailer))

	// get current idea of partition counts
	pc := c.partitionCache.lookup(pkey)

	// try once
	// Note: If missing from the cache, this sends "0,0" for counts, which the server will
	// always accept as not-stale if using dynamic scaling (but may reject for being invalid).
	// The first reply will have current counts if using scaling, or nothing if not.
	res, err := op(pc.appendToOutgoingContext(ctx), pc, request, opts)

	// update cache on trailer on both success and error. if the trailer has no data,
	// this removes the key from the cache.
	pc2, err2 := parsePartitionCountsFromTrailer(trailer)
	if err2 != nil {
		c.logger.Info("partition count trailer parse error", tag.Error(err2))
		// continue with zero value for pc2
	}
	if pc2 != pc {
		c.partitionCache.put(pkey, pc2)
	}

	if _, ok := errors.AsType[*serviceerrors.StalePartitionCounts](err); ok {
		// if we got a StalePartitionCounts, retry once
		trailer = nil
		res, err = op(pc2.appendToOutgoingContext(ctx), pc2, request, opts)
		// update again
		pc3, err3 := parsePartitionCountsFromTrailer(trailer)
		if err3 != nil {
			c.logger.Info("partition count trailer parse error", tag.Error(err3))
			// continue with zero value for pc3
		}
		if pc3 != pc2 {
			c.partitionCache.put(pkey, pc3)
		}
	}

	return res, err
}

func (c *clientImpl) AddActivityTask(
	ctx context.Context,
	request *matchingservice.AddActivityTaskRequest,
	opts ...grpc.CallOption,
) (*matchingservice.AddActivityTaskResponse, error) {
	pkey := c.partitionCache.makeKey(
		request.GetNamespaceId(),
		request.GetTaskQueue().GetName(),
		enumspb.TASK_QUEUE_TYPE_ACTIVITY,
	)
	kind := request.GetTaskQueue().GetKind()
	return handlePartitionCounts(ctx, c, pkey, kind, request, opts, c.addActivityTask)
}

func (c *clientImpl) addActivityTask(
	ctx context.Context,
	pc PartitionCounts,
	request *matchingservice.AddActivityTaskRequest,
	opts []grpc.CallOption,
) (*matchingservice.AddActivityTaskResponse, error) {
	request = common.CloneProto(request)
	client, err := c.pickClientForWrite(
		request.GetTaskQueue(),
		request.GetNamespaceId(),
		enumspb.TASK_QUEUE_TYPE_ACTIVITY,
		request.GetForwardInfo().GetSourcePartition(),
		pc,
	)
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()

	return client.AddActivityTask(ctx, request, opts...)
}

func (c *clientImpl) AddWorkflowTask(
	ctx context.Context,
	request *matchingservice.AddWorkflowTaskRequest,
	opts ...grpc.CallOption) (*matchingservice.AddWorkflowTaskResponse, error) {
	pkey := c.partitionCache.makeKey(
		request.GetNamespaceId(),
		request.GetTaskQueue().GetName(),
		enumspb.TASK_QUEUE_TYPE_WORKFLOW,
	)
	kind := request.GetTaskQueue().GetKind()
	return handlePartitionCounts(ctx, c, pkey, kind, request, opts, c.addWorkflowTask)
}

func (c *clientImpl) addWorkflowTask(
	ctx context.Context,
	pc PartitionCounts,
	request *matchingservice.AddWorkflowTaskRequest,
	opts []grpc.CallOption,
) (*matchingservice.AddWorkflowTaskResponse, error) {
	request = common.CloneProto(request)
	client, err := c.pickClientForWrite(
		request.GetTaskQueue(),
		request.GetNamespaceId(),
		enumspb.TASK_QUEUE_TYPE_WORKFLOW,
		request.GetForwardInfo().GetSourcePartition(),
		pc,
	)
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.AddWorkflowTask(ctx, request, opts...)
}

func (c *clientImpl) PollActivityTaskQueue(
	ctx context.Context,
	request *matchingservice.PollActivityTaskQueueRequest,
	opts ...grpc.CallOption,
) (*matchingservice.PollActivityTaskQueueResponse, error) {
	pkey := c.partitionCache.makeKey(
		request.GetNamespaceId(),
		request.GetPollRequest().GetTaskQueue().GetName(),
		enumspb.TASK_QUEUE_TYPE_ACTIVITY,
	)
	kind := request.GetPollRequest().GetTaskQueue().GetKind()
	return handlePartitionCounts(ctx, c, pkey, kind, request, opts, c.pollActivityTaskQueue)
}

func (c *clientImpl) pollActivityTaskQueue(
	ctx context.Context,
	pc PartitionCounts,
	request *matchingservice.PollActivityTaskQueueRequest,
	opts []grpc.CallOption,
) (*matchingservice.PollActivityTaskQueueResponse, error) {
	request = common.CloneProto(request)
	client, release, err := c.pickClientForRead(
		request.GetPollRequest().GetTaskQueue(),
		request.GetNamespaceId(),
		enumspb.TASK_QUEUE_TYPE_ACTIVITY,
		request.GetForwardedSource(),
		pc,
	)
	if err != nil {
		return nil, err
	}
	if release != nil {
		defer release()
	}
	ctx, cancel := c.createLongPollContext(ctx)
	defer cancel()
	return client.PollActivityTaskQueue(ctx, request, opts...)
}

func (c *clientImpl) PollWorkflowTaskQueue(
	ctx context.Context,
	request *matchingservice.PollWorkflowTaskQueueRequest,
	opts ...grpc.CallOption,
) (*matchingservice.PollWorkflowTaskQueueResponse, error) {
	pkey := c.partitionCache.makeKey(
		request.GetNamespaceId(),
		request.GetPollRequest().GetTaskQueue().GetName(),
		enumspb.TASK_QUEUE_TYPE_WORKFLOW,
	)
	kind := request.GetPollRequest().GetTaskQueue().GetKind()
	return handlePartitionCounts(ctx, c, pkey, kind, request, opts, c.pollWorkflowTaskQueue)
}

func (c *clientImpl) pollWorkflowTaskQueue(
	ctx context.Context,
	pc PartitionCounts,
	request *matchingservice.PollWorkflowTaskQueueRequest,
	opts []grpc.CallOption,
) (*matchingservice.PollWorkflowTaskQueueResponse, error) {
	request = common.CloneProto(request)
	client, release, err := c.pickClientForRead(
		request.GetPollRequest().GetTaskQueue(),
		request.GetNamespaceId(),
		enumspb.TASK_QUEUE_TYPE_WORKFLOW,
		request.GetForwardedSource(),
		pc,
	)
	if err != nil {
		return nil, err
	}
	if release != nil {
		defer release()
	}
	ctx, cancel := c.createLongPollContext(ctx)
	defer cancel()
	return client.PollWorkflowTaskQueue(ctx, request, opts...)
}

func (c *clientImpl) QueryWorkflow(
	ctx context.Context,
	request *matchingservice.QueryWorkflowRequest,
	opts ...grpc.CallOption,
) (*matchingservice.QueryWorkflowResponse, error) {
	pkey := c.partitionCache.makeKey(
		request.GetNamespaceId(),
		request.GetTaskQueue().GetName(),
		enumspb.TASK_QUEUE_TYPE_WORKFLOW,
	)
	kind := request.GetTaskQueue().GetKind()
	return handlePartitionCounts(ctx, c, pkey, kind, request, opts, c.queryWorkflow)
}

func (c *clientImpl) queryWorkflow(
	ctx context.Context,
	pc PartitionCounts,
	request *matchingservice.QueryWorkflowRequest,
	opts []grpc.CallOption,
) (*matchingservice.QueryWorkflowResponse, error) {
	// use shallow copy since QueryRequest may contain a large payload
	request = &matchingservice.QueryWorkflowRequest{
		NamespaceId:      request.NamespaceId,
		TaskQueue:        common.CloneProto(request.TaskQueue),
		QueryRequest:     request.QueryRequest,
		VersionDirective: request.VersionDirective,
		ForwardInfo:      request.ForwardInfo,
		Priority:         request.Priority,
	}
	client, err := c.pickClientForWrite(
		request.GetTaskQueue(),
		request.GetNamespaceId(),
		enumspb.TASK_QUEUE_TYPE_WORKFLOW,
		request.GetForwardInfo().GetSourcePartition(),
		pc,
	)
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.createContext(ctx)
	defer cancel()
	return client.QueryWorkflow(ctx, request, opts...)
}

// processInputPartition returns a partition in certain cases that load balancer involvement is not necessary,
// otherwise, returns a task queue to pass down to the load balancer.
func (c *clientImpl) processInputPartition(proto *taskqueuepb.TaskQueue, nsid string, taskType enumspb.TaskQueueType, forwardedFrom string) (tqid.Partition, *tqid.TaskQueue) {
	partition, err := tqid.PartitionFromProto(proto, nsid, taskType)
	if err != nil {
		// We preserve the old logic (not returning error in case of invalid proto info) until it's verified that
		// clients are not sending invalid names.
		c.logger.Info("invalid tq partition", tag.Error(err), tag.Stringer("proto", proto))
		metrics.MatchingClientInvalidTaskQueuePartition.With(c.metricsHandler).Record(1)
		return tqid.UnsafeTaskQueueFamily(nsid, proto.GetName()).TaskQueue(taskType).RootPartition(), nil
	}

	if forwardedFrom != "" || !partition.IsRoot() {
		return partition, nil
	}

	switch p := partition.(type) {
	case *tqid.NormalPartition:
		return nil, p.TaskQueue()
	default:
		return partition, nil
	}
}

// pickClientForWrite mutates the given proto. Callers should copy the proto before if necessary.
func (c *clientImpl) pickClientForWrite(
	proto *taskqueuepb.TaskQueue,
	nsid string,
	taskType enumspb.TaskQueueType,
	forwardedFrom string,
	pc PartitionCounts,
) (matchingservice.MatchingServiceClient, error) {
	p, tq := c.processInputPartition(proto, nsid, taskType, forwardedFrom)
	if tq != nil {
		p = c.loadBalancer.PickWritePartition(tq, pc)
	}
	proto.Name = p.RpcName()
	return c.getClientForTaskQueuePartition(p)
}

// pickClientForRead mutates the given proto. Callers should copy the proto before if necessary.
func (c *clientImpl) pickClientForRead(
	proto *taskqueuepb.TaskQueue,
	nsid string,
	taskType enumspb.TaskQueueType,
	forwardedFrom string,
	pc PartitionCounts,
) (client matchingservice.MatchingServiceClient, release func(), err error) {
	p, tq := c.processInputPartition(proto, nsid, taskType, forwardedFrom)
	if tq != nil {
		token := c.loadBalancer.PickReadPartition(tq, pc)
		p = token.TQPartition
		release = token.Release
	}

	proto.Name = p.RpcName()
	client, err = c.getClientForTaskQueuePartition(p)
	return client, release, err
}

func (c *clientImpl) createContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, c.timeout)
}

func (c *clientImpl) createLongPollContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, c.longPollTimeout)
}

func (c *clientImpl) Route(p tqid.Partition) (string, error) {
	spreadChange := c.spreadRouting()
	spread := spreadChange.Value(p.GradualChangeKey(), time.Now())
	return c.clients.Lookup(p.RoutingKey(spread))
}

func (c *clientImpl) getClientForTaskQueuePartition(
	partition tqid.Partition,
) (matchingservice.MatchingServiceClient, error) {
	addr, err := c.Route(partition)
	if err != nil {
		return nil, err
	}
	client, err := c.clients.GetClientForClientKey(addr)
	if err != nil {
		return nil, err
	}
	return client.(matchingservice.MatchingServiceClient), nil
}
