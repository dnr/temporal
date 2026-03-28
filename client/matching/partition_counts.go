package matching

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const partitionCountsHeaderName = "dpcount"
const partitionCountsTrailerName = "dpcount"

type PartitionCounts struct {
	Read, Write int32
}

func (pc PartitionCounts) Valid() bool {
	return pc.Read > 0 && pc.Write > 0
}

func (pc PartitionCounts) encode() string {
	return fmt.Sprintf("%d,%d", pc.Read, pc.Write)
}

func (pc PartitionCounts) appendToOutgoingContext(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, partitionCountsHeaderName, pc.encode())
}

func (pc PartitionCounts) SetTrailer(ctx context.Context) {
	grpc.SetTrailer(ctx, metadata.Pairs(partitionCountsTrailerName, pc.encode()))
}

func parsePartitionCounts(hdr string) (PartitionCounts, error) {
	parts := strings.SplitN(hdr, ",", 2)
	if len(parts) < 2 {
		return PartitionCounts{}, errors.New("not enough parts")
	}
	read, err := strconv.Atoi(parts[0])
	if err != nil {
		return PartitionCounts{}, err
	}
	write, err := strconv.Atoi(parts[1])
	if err != nil {
		return PartitionCounts{}, err
	}
	return PartitionCounts{
		Read:  int32(read),
		Write: int32(write),
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
