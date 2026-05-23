package semaphore

import (
	"testing"

	"github.com/stretchr/testify/require"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
)

// These tests exercise just the pure state-machine bits of Semaphore — the
// methods don't touch the chasm context, so we can pass nil.

func TestSetLimit_PromotesWaiters(t *testing.T) {
	s := newSemaphore(1)
	_, err := s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "a"})
	require.NoError(t, err)
	_, err = s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "b"})
	require.NoError(t, err)
	_, err = s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "c"})
	require.NoError(t, err)

	require.Equal(t, []string{"a"}, s.Holders)
	require.Equal(t, []string{"b", "c"}, s.Waiters)

	_, err = s.SetLimit(nil, &semaphorepb.SetLimitRequest{Limit: 3})
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, s.Holders)
	require.Empty(t, s.Waiters)
}

func TestAcquire_Idempotent(t *testing.T) {
	s := newSemaphore(1)
	r1, err := s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "a"})
	require.NoError(t, err)
	require.True(t, r1.acquired)

	r2, err := s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "a"})
	require.NoError(t, err)
	require.True(t, r2.acquired)
	require.Equal(t, []string{"a"}, s.Holders)
}

func TestAcquire_QueuesWhenFull(t *testing.T) {
	s := newSemaphore(1)
	_, _ = s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "a"})
	r, err := s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "b"})
	require.NoError(t, err)
	require.False(t, r.acquired)
	require.Equal(t, []string{"a"}, s.Holders)
	require.Equal(t, []string{"b"}, s.Waiters)
}

func TestRelease_PromotesNextWaiter(t *testing.T) {
	s := newSemaphore(1)
	_, _ = s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "a"})
	_, _ = s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "b"})
	_, _ = s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "c"})

	_, err := s.Release(nil, &semaphorepb.ReleaseRequest{HolderId: "a"})
	require.NoError(t, err)
	require.Equal(t, []string{"b"}, s.Holders)
	require.Equal(t, []string{"c"}, s.Waiters)
}

func TestRelease_NoOpForUnknown(t *testing.T) {
	s := newSemaphore(1)
	_, _ = s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "a"})
	_, err := s.Release(nil, &semaphorepb.ReleaseRequest{HolderId: "zzz"})
	require.NoError(t, err)
	require.Equal(t, []string{"a"}, s.Holders)
}

func TestRelease_RemovesQueuedWaiter(t *testing.T) {
	s := newSemaphore(1)
	_, _ = s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "a"})
	_, _ = s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "b"})
	_, _ = s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "c"})

	// Cancel b: should be removed from waiters, c still queued.
	_, err := s.Release(nil, &semaphorepb.ReleaseRequest{HolderId: "b"})
	require.NoError(t, err)
	require.Equal(t, []string{"a"}, s.Holders)
	require.Equal(t, []string{"c"}, s.Waiters)
}

func TestSetLimit_DecreaseDoesNotEvict(t *testing.T) {
	s := newSemaphore(3)
	_, _ = s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "a"})
	_, _ = s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "b"})
	_, _ = s.Enroll(nil, &semaphorepb.AcquireRequest{HolderId: "c"})

	_, err := s.SetLimit(nil, &semaphorepb.SetLimitRequest{Limit: 1})
	require.NoError(t, err)
	// Existing holders aren't kicked out; they keep their slot until they
	// Release. New Acquires will queue until holders drops below 1.
	require.Equal(t, []string{"a", "b", "c"}, s.Holders)
}

func TestRejects_NegativeLimit(t *testing.T) {
	s := newSemaphore(1)
	_, err := s.SetLimit(nil, &semaphorepb.SetLimitRequest{Limit: -1})
	require.Error(t, err)
}
