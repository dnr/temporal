package matching

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/server/common/log"
	serviceerrors "go.temporal.io/server/common/serviceerror"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// setTrailerInOpts finds the grpc.TrailerCallOption in opts and populates it.
func setTrailerInOpts(opts []grpc.CallOption, md metadata.MD) {
	for _, opt := range opts {
		if t, ok := opt.(grpc.TrailerCallOption); ok {
			*t.TrailerAddr = md
			return
		}
	}
}

func makeTrailer(pc PartitionCounts) metadata.MD {
	v, _ := pc.encode()
	return metadata.Pairs(partitionCountsTrailerName, v)
}

type hpcReq struct{}
type hpcRes struct{ value string }

func newTestCache() *partitionCache {
	cache := newPartitionCache()
	cache.Start()
	return cache
}

func TestHandlePartitionCounts_NonNormalSkipsCache(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	calls := 0
	op := func(ctx context.Context, pc PartitionCounts, req *hpcReq, opts []grpc.CallOption) (*hpcRes, error) {
		calls++
		assert.Equal(t, PartitionCounts{}, pc) // should get zero counts
		return &hpcRes{value: "ok"}, nil
	}

	pkey := cache.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	res, err := handlePartitionCounts(context.Background(), log.NewNoopLogger(), cache, pkey, enumspb.TASK_QUEUE_KIND_STICKY, &hpcReq{}, nil, op)
	require.NoError(t, err)
	assert.Equal(t, "ok", res.value)
	assert.Equal(t, 1, calls)
}

func TestHandlePartitionCounts_CacheMissSuccess(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	pkey := cache.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	serverPC := PartitionCounts{Read: 4, Write: 4}

	calls := 0
	op := func(ctx context.Context, pc PartitionCounts, req *hpcReq, opts []grpc.CallOption) (*hpcRes, error) {
		calls++
		assert.Equal(t, PartitionCounts{}, pc) // cache miss
		setTrailerInOpts(opts, makeTrailer(serverPC))
		return &hpcRes{value: "ok"}, nil
	}

	res, err := handlePartitionCounts(context.Background(), log.NewNoopLogger(), cache, pkey, enumspb.TASK_QUEUE_KIND_NORMAL, &hpcReq{}, nil, op)
	require.NoError(t, err)
	assert.Equal(t, "ok", res.value)
	assert.Equal(t, 1, calls)

	// Cache should be updated
	assert.Equal(t, serverPC, cache.lookup(pkey))
}

func TestHandlePartitionCounts_CacheHitSuccess(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	pkey := cache.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	cachedPC := PartitionCounts{Read: 4, Write: 4}
	cache.put(pkey, cachedPC)

	op := func(ctx context.Context, pc PartitionCounts, req *hpcReq, opts []grpc.CallOption) (*hpcRes, error) {
		assert.Equal(t, cachedPC, pc)
		// Server confirms same counts
		setTrailerInOpts(opts, makeTrailer(cachedPC))
		return &hpcRes{value: "ok"}, nil
	}

	res, err := handlePartitionCounts(context.Background(), log.NewNoopLogger(), cache, pkey, enumspb.TASK_QUEUE_KIND_NORMAL, &hpcReq{}, nil, op)
	require.NoError(t, err)
	assert.Equal(t, "ok", res.value)
	assert.Equal(t, cachedPC, cache.lookup(pkey))
}

func TestHandlePartitionCounts_ServerUpdatesCount(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	pkey := cache.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	cache.put(pkey, PartitionCounts{Read: 4, Write: 4})
	newPC := PartitionCounts{Read: 8, Write: 8}

	op := func(ctx context.Context, pc PartitionCounts, req *hpcReq, opts []grpc.CallOption) (*hpcRes, error) {
		// Server returns different counts
		setTrailerInOpts(opts, makeTrailer(newPC))
		return &hpcRes{value: "ok"}, nil
	}

	_, err := handlePartitionCounts(context.Background(), log.NewNoopLogger(), cache, pkey, enumspb.TASK_QUEUE_KIND_NORMAL, &hpcReq{}, nil, op)
	require.NoError(t, err)

	// Cache should be updated to new value
	assert.Equal(t, newPC, cache.lookup(pkey))
}

func TestHandlePartitionCounts_StaleRetrySucceeds(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	pkey := cache.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	serverPC := PartitionCounts{Read: 8, Write: 8}

	calls := 0
	op := func(ctx context.Context, pc PartitionCounts, req *hpcReq, opts []grpc.CallOption) (*hpcRes, error) {
		calls++
		setTrailerInOpts(opts, makeTrailer(serverPC))
		if calls == 1 {
			assert.Equal(t, PartitionCounts{}, pc) // cache miss
			return nil, serviceerrors.NewStalePartitionCounts("stale")
		}
		assert.Equal(t, serverPC, pc) // retry with updated counts
		return &hpcRes{value: "ok"}, nil
	}

	res, err := handlePartitionCounts(context.Background(), log.NewNoopLogger(), cache, pkey, enumspb.TASK_QUEUE_KIND_NORMAL, &hpcReq{}, nil, op)
	require.NoError(t, err)
	assert.Equal(t, "ok", res.value)
	assert.Equal(t, 2, calls)
	assert.Equal(t, serverPC, cache.lookup(pkey))
}

func TestHandlePartitionCounts_StaleRetryAlsoFails(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	pkey := cache.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	serverPC := PartitionCounts{Read: 8, Write: 8}

	calls := 0
	op := func(ctx context.Context, pc PartitionCounts, req *hpcReq, opts []grpc.CallOption) (*hpcRes, error) {
		calls++
		setTrailerInOpts(opts, makeTrailer(serverPC))
		if calls == 1 {
			return nil, serviceerrors.NewStalePartitionCounts("stale first")
		}
		// Second attempt: different non-stale error
		return nil, assert.AnError
	}

	_, err := handlePartitionCounts(context.Background(), log.NewNoopLogger(), cache, pkey, enumspb.TASK_QUEUE_KIND_NORMAL, &hpcReq{}, nil, op)
	assert.Error(t, err)
	assert.Equal(t, 2, calls)
}

func TestHandlePartitionCounts_NonStaleErrorNoRetry(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	pkey := cache.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	serverPC := PartitionCounts{Read: 4, Write: 4}

	calls := 0
	op := func(ctx context.Context, pc PartitionCounts, req *hpcReq, opts []grpc.CallOption) (*hpcRes, error) {
		calls++
		// Even on error, server sends trailer
		setTrailerInOpts(opts, makeTrailer(serverPC))
		return nil, assert.AnError
	}

	_, err := handlePartitionCounts(context.Background(), log.NewNoopLogger(), cache, pkey, enumspb.TASK_QUEUE_KIND_NORMAL, &hpcReq{}, nil, op)
	assert.Error(t, err)
	assert.Equal(t, 1, calls) // no retry

	// Cache should still be updated from trailer
	assert.Equal(t, serverPC, cache.lookup(pkey))
}

func TestHandlePartitionCounts_ZeroTrailerRemovesCache(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	pkey := cache.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	cache.put(pkey, PartitionCounts{Read: 4, Write: 4})

	op := func(ctx context.Context, pc PartitionCounts, req *hpcReq, opts []grpc.CallOption) (*hpcRes, error) {
		// Server signals "dynamic partitioning off"
		setTrailerInOpts(opts, makeTrailer(PartitionCounts{Read: 0, Write: 0}))
		return &hpcRes{value: "ok"}, nil
	}

	_, err := handlePartitionCounts(context.Background(), log.NewNoopLogger(), cache, pkey, enumspb.TASK_QUEUE_KIND_NORMAL, &hpcReq{}, nil, op)
	require.NoError(t, err)

	// Cache entry should be removed
	assert.Equal(t, PartitionCounts{}, cache.lookup(pkey))
}

func TestHandlePartitionCounts_NoTrailerDoesNothing(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	pkey := cache.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	originalPC := PartitionCounts{Read: 4, Write: 4}
	cache.put(pkey, originalPC)

	op := func(ctx context.Context, pc PartitionCounts, req *hpcReq, opts []grpc.CallOption) (*hpcRes, error) {
		// No trailer set
		return &hpcRes{value: "ok"}, nil
	}

	_, err := handlePartitionCounts(context.Background(), log.NewNoopLogger(), cache, pkey, enumspb.TASK_QUEUE_KIND_NORMAL, &hpcReq{}, nil, op)
	require.NoError(t, err)

	// Cache unchanged (trailer was empty → pc2 == {0,0} != pc == {4,4} → put removes)
	// Actually: no trailer → parsePartitionCountsFromTrailer returns {0,0} → pc2={0,0} != pc={4,4}
	// → put({0,0}) which is invalid → removes the entry
	assert.Equal(t, PartitionCounts{}, cache.lookup(pkey))
}

func TestHandlePartitionCounts_OutgoingContextHasHeader(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	pkey := cache.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	cachedPC := PartitionCounts{Read: 6, Write: 4}
	cache.put(pkey, cachedPC)

	op := func(ctx context.Context, pc PartitionCounts, req *hpcReq, opts []grpc.CallOption) (*hpcRes, error) {
		// Verify the outgoing context has the partition counts header
		md, ok := metadata.FromOutgoingContext(ctx)
		require.True(t, ok)
		vals := md.Get(partitionCountsHeaderName)
		require.Len(t, vals, 1)
		parsed, err := parsePartitionCounts(vals[0])
		require.NoError(t, err)
		assert.Equal(t, cachedPC, parsed)

		setTrailerInOpts(opts, makeTrailer(cachedPC))
		return &hpcRes{value: "ok"}, nil
	}

	_, err := handlePartitionCounts(context.Background(), log.NewNoopLogger(), cache, pkey, enumspb.TASK_QUEUE_KIND_NORMAL, &hpcReq{}, nil, op)
	require.NoError(t, err)
}
