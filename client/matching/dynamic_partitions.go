package matching

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/grpc/metadata"
)

const partitionCountsHeaderName = "dpcount"
const partitionCountsTrailerName = "dpcount"

type partitionCounts struct {
	read, write int16
}

func (pc partitionCounts) valid() bool {
	return pc.read > 0 && pc.write > 0
}

func (pc partitionCounts) encode() string {
	return fmt.Sprintf("%d,%d", pc.read, pc.write)
}

func (pc partitionCounts) appendToOutgoingContext(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, partitionCountsHeaderName, pc.encode())
}

func parsePartitionCounts(hdr string) (partitionCounts, error) {
	parts := strings.SplitN(hdr, ",", 2)
	if len(parts) < 2 {
		return partitionCounts{}, errors.New("not enough parts")
	}
	read, err := strconv.Atoi(parts[0])
	if err != nil {
		return partitionCounts{}, err
	}
	write, err := strconv.Atoi(parts[1])
	if err != nil {
		return partitionCounts{}, err
	}
	return partitionCounts{
		read:  int16(read),
		write: int16(write),
	}, nil
}

func parsePartitionCountsFromIncomingContext(ctx context.Context) (partitionCounts, error) {
	vals := metadata.ValueFromIncomingContext(ctx, partitionCountsHeaderName)
	if len(vals) == 0 {
		return partitionCounts{}, nil
	}
	return parsePartitionCounts(vals[0])
}

func parsePartitionCountsFromTrailer(trailer metadata.MD) (partitionCounts, error) {
	vals := trailer.Get(partitionCountsTrailerName)
	if len(vals) == 0 {
		return partitionCounts{}, nil
	}
	return parsePartitionCounts(vals[0])
}
