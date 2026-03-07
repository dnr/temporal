package matching

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.temporal.io/api/serviceerror"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	matching "go.temporal.io/server/client/matching"
	serviceerrors "go.temporal.io/server/common/serviceerror"
)

func TestValidatePartitionCounts_NilScaleInfo(t *testing.T) {
	t.Parallel()
	err := validatePartitionCounts(0, nil, matching.PartitionCounts{}, true, 2, 1.5)
	assert.NoError(t, err)
}

func TestValidatePartitionCounts_ZeroScaleInfo(t *testing.T) {
	t.Parallel()
	err := validatePartitionCounts(0, &taskqueuespb.PartitionScaleInfo{Read: 0, Write: 0}, matching.PartitionCounts{}, true, 2, 1.5)
	assert.NoError(t, err)
}

func TestValidatePartitionCounts_InvalidScaleInfo_WriteGtRead(t *testing.T) {
	t.Parallel()
	err := validatePartitionCounts(0, &taskqueuespb.PartitionScaleInfo{Read: 2, Write: 4}, matching.PartitionCounts{}, true, 2, 1.5)
	assert.NoError(t, err)
}

func TestValidatePartitionCounts_NegativePartitionID(t *testing.T) {
	t.Parallel()
	si := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 4}
	err := validatePartitionCounts(-1, si, matching.PartitionCounts{}, true, 2, 1.5)
	var internal *serviceerror.Internal
	assert.ErrorAs(t, err, &internal)
}

func TestValidatePartitionCounts_InvalidPartition(t *testing.T) {
	t.Parallel()
	si := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 4}

	// id == read → invalid
	err := validatePartitionCounts(8, si, matching.PartitionCounts{}, true, 2, 1.5)
	var stale *serviceerrors.StalePartitionCounts
	assert.ErrorAs(t, err, &stale)

	// id > read → invalid
	err = validatePartitionCounts(10, si, matching.PartitionCounts{}, false, 2, 1.5)
	assert.ErrorAs(t, err, &stale)
}

func TestValidatePartitionCounts_DrainingPartition_Write(t *testing.T) {
	t.Parallel()
	// Read=8, Write=4. Partition 5 is draining (>= write, < read).
	si := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 4}

	// forWrite on draining partition → reject
	err := validatePartitionCounts(5, si, matching.PartitionCounts{}, true, 2, 1.5)
	var stale *serviceerrors.StalePartitionCounts
	assert.ErrorAs(t, err, &stale)
}

func TestValidatePartitionCounts_DrainingPartition_Read(t *testing.T) {
	t.Parallel()
	// Read=8, Write=4. Partition 5 is draining.
	si := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 4}

	// forWrite=false (poll) on draining partition → accept
	err := validatePartitionCounts(5, si, matching.PartitionCounts{}, false, 2, 1.5)
	assert.NoError(t, err)
}

func TestValidatePartitionCounts_ActivePartition(t *testing.T) {
	t.Parallel()
	si := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 4}

	// Partition 2 is active (< write)
	err := validatePartitionCounts(2, si, matching.PartitionCounts{}, true, 2, 1.5)
	assert.NoError(t, err)
	err = validatePartitionCounts(2, si, matching.PartitionCounts{}, false, 2, 1.5)
	assert.NoError(t, err)
}

func TestValidatePartitionCounts_NoClientCounts(t *testing.T) {
	t.Parallel()
	si := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 8}

	// No client counts (zero) → accept even though partition is valid
	err := validatePartitionCounts(3, si, matching.PartitionCounts{}, true, 2, 1.5)
	assert.NoError(t, err)
}

func TestValidatePartitionCounts_ClientCountsMatch(t *testing.T) {
	t.Parallel()
	si := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 8}
	clientPC := matching.PartitionCounts{Read: 8, Write: 8}

	err := validatePartitionCounts(3, si, clientPC, true, 2, 1.5)
	assert.NoError(t, err)
}

func TestValidatePartitionCounts_ClientCountsTooFarOff(t *testing.T) {
	t.Parallel()
	si := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 8}

	// Write: Client thinks 20, server has 8. Delta = 12 > 2, Ratio = 20/8 = 2.5 > 1.5
	clientPC := matching.PartitionCounts{Read: 20, Write: 20}
	err := validatePartitionCounts(3, si, clientPC, true, 2, 1.5)
	var stale *serviceerrors.StalePartitionCounts
	assert.ErrorAs(t, err, &stale)
}

func TestValidatePartitionCounts_ClientCountsWithinDelta(t *testing.T) {
	t.Parallel()
	si := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 8}

	// Delta: 10 - 8 = 2, within allowed delta of 2
	// Even though ratio is 10/8 = 1.25 which might exceed ratio, delta is within
	clientPC := matching.PartitionCounts{Read: 10, Write: 10}
	err := validatePartitionCounts(3, si, clientPC, true, 2, 1.1)
	assert.NoError(t, err) // delta within → allowed
}

func TestValidatePartitionCounts_ClientCountsWithinRatio(t *testing.T) {
	t.Parallel()
	si := &taskqueuespb.PartitionScaleInfo{Read: 100, Write: 100}

	// Delta: 110 - 100 = 10, exceeds allowed delta of 2
	// Ratio: 110/100 = 1.1, within allowed ratio of 1.5
	clientPC := matching.PartitionCounts{Read: 110, Write: 110}
	err := validatePartitionCounts(3, si, clientPC, true, 2, 1.5)
	assert.NoError(t, err) // ratio within → allowed
}

func TestValidatePartitionCounts_ClientCountsBothExceed(t *testing.T) {
	t.Parallel()
	si := &taskqueuespb.PartitionScaleInfo{Read: 4, Write: 4}

	// Delta: 10 - 4 = 6 > 2. Ratio: 10/4 = 2.5 > 1.5. Both exceed.
	clientPC := matching.PartitionCounts{Read: 10, Write: 10}
	err := validatePartitionCounts(0, si, clientPC, true, 2, 1.5)
	var stale *serviceerrors.StalePartitionCounts
	assert.ErrorAs(t, err, &stale)
}

func TestValidatePartitionCounts_ReadPath_UsesReadCounts(t *testing.T) {
	t.Parallel()
	si := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 4}

	// forWrite=false: should compare client.Read vs server.Read
	// Client Read=20, Server Read=8. Delta=12 > 2, Ratio=2.5 > 1.5 → reject
	clientPC := matching.PartitionCounts{Read: 20, Write: 4}
	err := validatePartitionCounts(0, si, clientPC, false, 2, 1.5)
	var stale *serviceerrors.StalePartitionCounts
	assert.ErrorAs(t, err, &stale)
}

func TestValidatePartitionCounts_WritePath_UsesWriteCounts(t *testing.T) {
	t.Parallel()
	si := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 4}

	// forWrite=true: should compare client.Write vs server.Write
	// Client Write=4, Server Write=4 → match → accept. Even though Read differs.
	clientPC := matching.PartitionCounts{Read: 20, Write: 4}
	err := validatePartitionCounts(0, si, clientPC, true, 2, 1.5)
	assert.NoError(t, err)
}

func TestValidatePartitionCounts_ClientCountsBelow(t *testing.T) {
	t.Parallel()
	si := &taskqueuespb.PartitionScaleInfo{Read: 20, Write: 20}

	// Client thinks fewer partitions. Delta = -16, abs = 16 > 2. Ratio = 4/20 = 0.2, 1/0.2 = 5 > 1.5
	clientPC := matching.PartitionCounts{Read: 4, Write: 4}
	err := validatePartitionCounts(0, si, clientPC, true, 2, 1.5)
	var stale *serviceerrors.StalePartitionCounts
	assert.ErrorAs(t, err, &stale)
}
