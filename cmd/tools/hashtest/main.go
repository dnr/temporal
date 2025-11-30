package main

import (
	"flag"
	"fmt"
	"math"
	"sort"

	"github.com/dgryski/go-farm"
	"github.com/temporalio/ringpop-go/hashring"
	"github.com/temporalio/ringpop-go/membership"
)

var (
	flagN             = flag.Int("n", 10, "number of nodes")
	flagK             = flag.Int("k", 5, "number of service parts to place")
	flagReplicaPoints = flag.Int("replica-points", 100, "replica points for consistent hash ring")
	flagSimulations   = flag.Int("sims", 1000, "number of simulations to run")
	flagBatchSize     = flag.Int("batch", 0, "batch size for batched variants (0 = skip batched tests)")
)

type simResult struct {
	uniqueNodes int
	maxLoad     int
	minLoad     int // min load among used nodes
	stdDev      float64
}

type removalResult struct {
	moved int // number of placements that changed
}

type simStats struct {
	avgUniqueNodes float64
	avgMaxLoad     float64
	avgMinLoad     float64
	avgStdDev      float64
	// Distribution of unique node counts
	uniqueNodesDist map[int]int
}

type removalStats struct {
	avgMoved float64
	// Distribution of moved counts
	movedDist map[int]int
}

func main() {
	flag.Parse()

	n := *flagN
	k := *flagK
	replicaPoints := *flagReplicaPoints
	numSims := *flagSimulations
	batchSize := *flagBatchSize

	fmt.Printf("Configuration: N=%d nodes, K=%d parts, ReplicaPoints=%d, Simulations=%d", n, k, replicaPoints, numSims)
	if batchSize > 0 {
		fmt.Printf(", BatchSize=%d", batchSize)
	}
	fmt.Printf("\n\n")

	// Run simulations
	consistentResults := make([]simResult, numSims)
	rendezvousResults := make([]simResult, numSims)
	consistentNResults := make([]simResult, numSims)
	rendezvousNResults := make([]simResult, numSims)

	// Removal simulations
	consistentRemoval := make([]removalResult, numSims)
	rendezvousRemoval := make([]removalResult, numSims)
	consistentNRemoval := make([]removalResult, numSims)
	rendezvousNRemoval := make([]removalResult, numSims)

	// Batched variants (only if batchSize > 0)
	var consistentBatchResults, rendezvousBatchResults []simResult
	var consistentBatchRemoval, rendezvousBatchRemoval []removalResult
	if batchSize > 0 {
		consistentBatchResults = make([]simResult, numSims)
		rendezvousBatchResults = make([]simResult, numSims)
		consistentBatchRemoval = make([]removalResult, numSims)
		rendezvousBatchRemoval = make([]removalResult, numSims)
	}

	for sim := 0; sim < numSims; sim++ {
		// Generate unique node names for this simulation
		nodes := make([]string, n)
		for i := range nodes {
			nodes[i] = fmt.Sprintf("sim%d-node-%d", sim, i)
		}
		serviceName := fmt.Sprintf("sim%d-service", sim)

		ring := buildRing(nodes, replicaPoints)

		consistentPlacements := testConsistentHash(ring, k, serviceName)
		consistentResults[sim] = computeResult(consistentPlacements, n)

		rendezvousPlacements := testRendezvousHash(nodes, k, serviceName)
		rendezvousResults[sim] = computeResult(rendezvousPlacements, n)

		consistentNPlacements := testConsistentHashN(ring, k, serviceName)
		consistentNResults[sim] = computeResult(consistentNPlacements, n)

		rendezvousNPlacements := testRendezvousHashN(nodes, k, serviceName)
		rendezvousNResults[sim] = computeResult(rendezvousNPlacements, n)

		// Now remove node 0 and see how placements change
		nodesAfter := nodes[1:]
		ringAfter := buildRing(nodesAfter, replicaPoints)

		consistentAfter := testConsistentHash(ringAfter, k, serviceName)
		consistentRemoval[sim] = computeRemovalResult(consistentPlacements, consistentAfter)

		rendezvousAfter := testRendezvousHash(nodesAfter, k, serviceName)
		rendezvousRemoval[sim] = computeRemovalResult(rendezvousPlacements, rendezvousAfter)

		consistentNAfter := testConsistentHashN(ringAfter, k, serviceName)
		consistentNRemoval[sim] = computeRemovalResult(consistentNPlacements, consistentNAfter)

		rendezvousNAfter := testRendezvousHashN(nodesAfter, k, serviceName)
		rendezvousNRemoval[sim] = computeRemovalResult(rendezvousNPlacements, rendezvousNAfter)

		// Batched variants
		if batchSize > 0 {
			consistentBatchPlacements := testConsistentHashBatch(ring, k, batchSize, serviceName)
			consistentBatchResults[sim] = computeResult(consistentBatchPlacements, n)

			rendezvousBatchPlacements := testRendezvousHashBatch(nodes, k, batchSize, serviceName)
			rendezvousBatchResults[sim] = computeResult(rendezvousBatchPlacements, n)

			consistentBatchAfter := testConsistentHashBatch(ringAfter, k, batchSize, serviceName)
			consistentBatchRemoval[sim] = computeRemovalResult(consistentBatchPlacements, consistentBatchAfter)

			rendezvousBatchAfter := testRendezvousHashBatch(nodesAfter, k, batchSize, serviceName)
			rendezvousBatchRemoval[sim] = computeRemovalResult(rendezvousBatchPlacements, rendezvousBatchAfter)
		}
	}

	// Compute and print statistics
	fmt.Println("=== Consistent Hashing (independent lookups) ===")
	consistentStats := computeStats(consistentResults)
	printStats(consistentStats, n, k)
	printRemovalStats(computeRemovalStats(consistentRemoval), k)

	fmt.Println()

	fmt.Println("=== Rendezvous Hashing (independent lookups) ===")
	rendezvousStats := computeStats(rendezvousResults)
	printStats(rendezvousStats, n, k)
	printRemovalStats(computeRemovalStats(rendezvousRemoval), k)

	fmt.Println()

	fmt.Println("=== Consistent Hashing (LookupN) ===")
	consistentNStats := computeStats(consistentNResults)
	printStats(consistentNStats, n, k)
	printRemovalStats(computeRemovalStats(consistentNRemoval), k)

	fmt.Println()

	fmt.Println("=== Rendezvous Hashing (top-K) ===")
	rendezvousNStats := computeStats(rendezvousNResults)
	printStats(rendezvousNStats, n, k)
	printRemovalStats(computeRemovalStats(rendezvousNRemoval), k)

	if batchSize > 0 {
		fmt.Println()

		fmt.Printf("=== Consistent Hashing (batched, B=%d) ===\n", batchSize)
		consistentBatchStats := computeStats(consistentBatchResults)
		printStats(consistentBatchStats, n, k)
		printRemovalStats(computeRemovalStats(consistentBatchRemoval), k)

		fmt.Println()

		fmt.Printf("=== Rendezvous Hashing (batched, B=%d) ===\n", batchSize)
		rendezvousBatchStats := computeStats(rendezvousBatchResults)
		printStats(rendezvousBatchStats, n, k)
		printRemovalStats(computeRemovalStats(rendezvousBatchRemoval), k)
	}
}

// simpleMember implements membership.Member for the HashRing
type simpleMember struct {
	address string
}

func (m simpleMember) GetAddress() string {
	return m.address
}

func (m simpleMember) Label(key string) (string, bool) {
	return "", false
}

func (m simpleMember) Identity() string {
	return m.address
}

var _ membership.Member = simpleMember{}

func buildRing(nodes []string, replicaPoints int) *hashring.HashRing {
	ring := hashring.New(farm.Fingerprint32, replicaPoints)
	members := make([]membership.Member, len(nodes))
	for i, node := range nodes {
		members[i] = simpleMember{address: node}
	}
	ring.AddMembers(members...)
	return ring
}

// testConsistentHash does K independent lookups
func testConsistentHash(ring *hashring.HashRing, k int, serviceName string) []string {
	placements := make([]string, k)
	for i := 0; i < k; i++ {
		key := fmt.Sprintf("%s-%d", serviceName, i)
		node, ok := ring.Lookup(key)
		if !ok {
			panic("lookup failed")
		}
		placements[i] = node
	}
	return placements
}

// testConsistentHashN uses LookupN to get K distinct nodes (cycling if K > N)
func testConsistentHashN(ring *hashring.HashRing, k int, serviceName string) []string {
	nodes := ring.LookupN(serviceName, k)
	if len(nodes) >= k {
		return nodes[:k]
	}
	// K > N: cycle through the nodes
	result := make([]string, k)
	for i := 0; i < k; i++ {
		result[i] = nodes[i%len(nodes)]
	}
	return result
}

// testRendezvousHash does K independent lookups
func testRendezvousHash(nodes []string, k int, serviceName string) []string {
	placements := make([]string, k)
	for i := 0; i < k; i++ {
		key := fmt.Sprintf("%s-%d", serviceName, i)
		placements[i] = rendezvousLookup(nodes, key)
	}
	return placements
}

// rendezvousLookup finds the node with highest hash for the given key
func rendezvousLookup(nodes []string, key string) string {
	var bestNode string
	var bestHash uint32 = 0

	for _, node := range nodes {
		// Combine node and key, then hash
		combined := node + ":" + key
		h := farm.Fingerprint32([]byte(combined))
		if h > bestHash {
			bestHash = h
			bestNode = node
		}
	}
	return bestNode
}

// testRendezvousHashN picks top K nodes by hash value
func testRendezvousHashN(nodes []string, k int, serviceName string) []string {
	type nodeHash struct {
		node string
		hash uint32
	}

	hashes := make([]nodeHash, len(nodes))
	for i, node := range nodes {
		combined := node + ":" + serviceName
		hashes[i] = nodeHash{node: node, hash: farm.Fingerprint32([]byte(combined))}
	}

	// Sort by hash descending
	sort.Slice(hashes, func(i, j int) bool {
		return hashes[i].hash > hashes[j].hash
	})

	// Take top K
	result := make([]string, k)
	for i := 0; i < k; i++ {
		result[i] = hashes[i%len(hashes)].node
	}
	return result
}

// testConsistentHashBatch divides K parts into batches of size B.
// For each batch, uses LookupN to get B distinct nodes.
func testConsistentHashBatch(ring *hashring.HashRing, k, batchSize int, serviceName string) []string {
	result := make([]string, k)
	for batchStart := 0; batchStart < k; batchStart += batchSize {
		batchEnd := batchStart + batchSize
		if batchEnd > k {
			batchEnd = k
		}
		batchKey := fmt.Sprintf("%s-%d", serviceName, batchStart)
		batchNodes := ring.LookupN(batchKey, batchSize)

		for i := batchStart; i < batchEnd; i++ {
			result[i] = batchNodes[(i-batchStart)%len(batchNodes)]
		}
	}
	return result
}

// testRendezvousHashBatch divides K parts into batches of size B.
// For each batch, picks top B nodes by rendezvous hash.
func testRendezvousHashBatch(nodes []string, k, batchSize int, serviceName string) []string {
	type nodeHash struct {
		node string
		hash uint32
	}

	result := make([]string, k)
	for batchStart := 0; batchStart < k; batchStart += batchSize {
		batchEnd := batchStart + batchSize
		if batchEnd > k {
			batchEnd = k
		}
		batchKey := fmt.Sprintf("%s-%d", serviceName, batchStart)

		// Compute rendezvous scores for this batch
		hashes := make([]nodeHash, len(nodes))
		for i, node := range nodes {
			combined := node + ":" + batchKey
			hashes[i] = nodeHash{node: node, hash: farm.Fingerprint32([]byte(combined))}
		}

		// Sort by hash descending
		sort.Slice(hashes, func(i, j int) bool {
			return hashes[i].hash > hashes[j].hash
		})

		// Assign parts in this batch to top-B nodes
		for i := batchStart; i < batchEnd; i++ {
			idx := (i - batchStart) % len(hashes)
			if idx >= batchSize && batchSize <= len(hashes) {
				idx = idx % batchSize
			}
			result[i] = hashes[idx].node
		}
	}
	return result
}

func computeResult(placements []string, numNodes int) simResult {
	// Count per node
	counts := make(map[string]int)
	for _, p := range placements {
		counts[p]++
	}

	uniqueNodes := len(counts)
	maxLoad := 0
	minLoad := len(placements)
	for _, c := range counts {
		if c > maxLoad {
			maxLoad = c
		}
		if c < minLoad {
			minLoad = c
		}
	}

	// Calculate standard deviation from expected
	expectedPerNode := float64(len(placements)) / float64(numNodes)
	var sumSq float64
	for _, c := range counts {
		diff := float64(c) - expectedPerNode
		sumSq += diff * diff
	}
	// Also count nodes with 0 load
	for i := 0; i < numNodes-len(counts); i++ {
		sumSq += expectedPerNode * expectedPerNode
	}
	stdDev := math.Sqrt(sumSq / float64(numNodes))

	return simResult{
		uniqueNodes: uniqueNodes,
		maxLoad:     maxLoad,
		minLoad:     minLoad,
		stdDev:      stdDev,
	}
}

func computeStats(results []simResult) simStats {
	stats := simStats{
		uniqueNodesDist: make(map[int]int),
	}

	for _, r := range results {
		stats.avgUniqueNodes += float64(r.uniqueNodes)
		stats.avgMaxLoad += float64(r.maxLoad)
		stats.avgMinLoad += float64(r.minLoad)
		stats.avgStdDev += r.stdDev
		stats.uniqueNodesDist[r.uniqueNodes]++
	}

	n := float64(len(results))
	stats.avgUniqueNodes /= n
	stats.avgMaxLoad /= n
	stats.avgMinLoad /= n
	stats.avgStdDev /= n

	return stats
}

func printStats(stats simStats, numNodes, k int) {
	expectedPerNode := float64(k) / float64(numNodes)

	fmt.Printf("Average unique nodes used: %.2f / %d\n", stats.avgUniqueNodes, numNodes)
	fmt.Printf("Average max load on any node: %.2f\n", stats.avgMaxLoad)
	fmt.Printf("Average min load on used nodes: %.2f\n", stats.avgMinLoad)
	fmt.Printf("Expected load per node: %.2f\n", expectedPerNode)
	fmt.Printf("Average std dev from expected: %.2f\n", stats.avgStdDev)

	// Print distribution of unique node counts
	fmt.Printf("Distribution of unique nodes used:\n")
	maxUnique := 0
	for u := range stats.uniqueNodesDist {
		if u > maxUnique {
			maxUnique = u
		}
	}
	for u := 1; u <= maxUnique; u++ {
		if count, ok := stats.uniqueNodesDist[u]; ok {
			pct := float64(count) / float64(sumValues(stats.uniqueNodesDist)) * 100
			fmt.Printf("  %d nodes: %d (%.1f%%)\n", u, count, pct)
		}
	}
}

func sumValues(m map[int]int) int {
	sum := 0
	for _, v := range m {
		sum += v
	}
	return sum
}

func computeRemovalResult(before, after []string) removalResult {
	moved := 0
	for i := range before {
		if before[i] != after[i] {
			moved++
		}
	}
	return removalResult{moved: moved}
}

func computeRemovalStats(results []removalResult) removalStats {
	stats := removalStats{
		movedDist: make(map[int]int),
	}
	for _, r := range results {
		stats.avgMoved += float64(r.moved)
		stats.movedDist[r.moved]++
	}
	stats.avgMoved /= float64(len(results))
	return stats
}

func printRemovalStats(stats removalStats, k int) {
	fmt.Printf("On node removal: avg %.2f / %d placements move\n", stats.avgMoved, k)
	fmt.Printf("Distribution of moves:\n")
	maxMoved := 0
	for m := range stats.movedDist {
		if m > maxMoved {
			maxMoved = m
		}
	}
	for m := 0; m <= maxMoved; m++ {
		if count, ok := stats.movedDist[m]; ok {
			pct := float64(count) / float64(sumValues(stats.movedDist)) * 100
			fmt.Printf("  %d moved: %d (%.1f%%)\n", m, count, pct)
		}
	}
}
