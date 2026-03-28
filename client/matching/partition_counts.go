package matching

import (
	"context"
	"errors"
	"slices"

	"github.com/gogo/protobuf/proto"
	enumspb "go.temporal.io/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common/log/tag"
	serviceerrors "go.temporal.io/server/common/serviceerror"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// The "-bin" suffix instructs grpc to base64-encode the value, so we can use binary.
const partitionCountsHeaderName = "pcnt-bin"
const partitionCountsTrailerName = "pcnt-bin"

// PartitionCounts is a smaller version of taskqueuespb.ClientPartitionCounts that we can more
// easily pass around and put in a map.
type PartitionCounts struct {
	Read, Write int32
}

func (pc PartitionCounts) Valid() bool {
	return pc.Read > 0 && pc.Write > 0
}

func (pc PartitionCounts) encode() (string, error) {
	b, err := proto.Marshal(&taskqueuespb.ClientPartitionCounts{
		Read:  pc.Read,
		Write: pc.Write,
	})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (pc PartitionCounts) appendToOutgoingContext(ctx context.Context) context.Context {
	v, err := pc.encode()
	if err != nil {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, partitionCountsHeaderName, v)
}

func (pc PartitionCounts) SetTrailer(ctx context.Context) {
	v, err := pc.encode()
	if err != nil {
		return
	}
	grpc.SetTrailer(ctx, metadata.Pairs(partitionCountsTrailerName, v))
}

func parsePartitionCounts(hdr string) (PartitionCounts, error) {
	var cpc taskqueuespb.ClientPartitionCounts
	err := proto.Unmarshal([]byte(hdr), &cpc)
	if err != nil {
		return PartitionCounts{}, err
	}
	return PartitionCounts{
		Read:  cpc.Read,
		Write: cpc.Write,
	}, nil
}

func ParsePartitionCountsFromIncomingContext(ctx context.Context) (PartitionCounts, error) {
	vals := metadata.ValueFromIncomingContext(ctx, partitionCountsHeaderName)
	if len(vals) == 0 {
		return PartitionCounts{}, nil
	}
	return parsePartitionCounts(vals[0])
}

func parsePartitionCountsFromTrailer(trailer metadata.MD) (PartitionCounts, error) {
	vals := trailer.Get(partitionCountsTrailerName)
	if len(vals) == 0 {
		return PartitionCounts{}, nil
	}
	return parsePartitionCounts(vals[0])
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
