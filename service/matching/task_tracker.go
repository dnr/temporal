package matching

import (
	"sync"
	"time"

	"go.temporal.io/server/common/clock"
)

// a circular array of a fixed size for tracking tasks
type circularTaskBuffer struct {
	buffer     []int32
	currentPos int
}

func newCircularTaskBuffer(size int) circularTaskBuffer {
	return circularTaskBuffer{
		buffer: make([]int32, size),
	}
}

func (cb *circularTaskBuffer) inc(n int) {
	cb.buffer[cb.currentPos] += int32(n)
}

func (cb *circularTaskBuffer) advance() {
	cb.currentPos = (cb.currentPos + 1) % len(cb.buffer)
	cb.buffer[cb.currentPos] = 0 // Reset the task count for the new interval
}

// returns the total number of tasks in the buffer
func (cb *circularTaskBuffer) totalTasks() int {
	totalTasks := 0
	for _, count := range cb.buffer {
		totalTasks += int(count)
	}
	return totalTasks
}

type taskTracker struct {
	lock              sync.Mutex
	clock             clock.TimeSource
	startTime         time.Time     // time when taskTracker was initialized
	bucketStartTime   time.Time     // the starting time of a bucket in the buffer
	intervalSize      time.Duration // the duration of each bucket in the buffer
	buckets           int           // the total number of buckets in the buffer
	totalIntervalSize time.Duration // the number of seconds over which rate of tasks are added/dispatched
	tasksInInterval   circularTaskBuffer
}

func newTaskTracker(
	timeSource clock.TimeSource,
	intervalSize time.Duration,
	totalIntervalSize time.Duration,
) *taskTracker {
	buckets := int(totalIntervalSize/intervalSize) + 1
	return &taskTracker{
		clock:             timeSource,
		startTime:         timeSource.Now(),
		bucketStartTime:   timeSource.Now(),
		intervalSize:      intervalSize,
		buckets:           buckets,
		totalIntervalSize: totalIntervalSize,
		tasksInInterval:   newCircularTaskBuffer(buckets),
	}
}

// advanceAndResetLocked advances the trackers position and clears out any expired intervals
// This method must be called with taskTracker's lock held.
func (s *taskTracker) advanceAndResetLocked(elapsed time.Duration) {
	// Calculate the number of intervals elapsed since the start interval time
	intervalsElapsed := int(elapsed / s.intervalSize)

	for i := 0; i < min(intervalsElapsed, s.buckets); i++ {
		s.tasksInInterval.advance() // advancing our circular buffer's position until we land on the right interval
	}
	s.bucketStartTime = s.bucketStartTime.Add(time.Duration(intervalsElapsed) * s.intervalSize)
}

// inc adds/removes tasks from the current time that falls in the appropriate interval
func (s *taskTracker) inc(n int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	currentTime := s.clock.Now()

	// Calculate elapsed time from the latest start interval time
	elapsed := currentTime.Sub(s.bucketStartTime)
	s.advanceAndResetLocked(elapsed)
	s.tasksInInterval.inc(n)
}

// rate returns the rate of tasks added/dispatched in a given interval
func (s *taskTracker) rate() float32 {
	s.lock.Lock()
	defer s.lock.Unlock()
	currentTime := s.clock.Now()

	// Calculate elapsed time from the latest start interval time
	elapsed := currentTime.Sub(s.bucketStartTime)
	s.advanceAndResetLocked(elapsed)
	totalTasks := s.tasksInInterval.totalTasks()

	elapsedTime := min(currentTime.Sub(s.bucketStartTime)+s.totalIntervalSize,
		currentTime.Sub(s.startTime))

	if elapsedTime <= 0 {
		return 0
	}

	// rate per second
	return float32(totalTasks) / float32(elapsedTime.Seconds())
}
