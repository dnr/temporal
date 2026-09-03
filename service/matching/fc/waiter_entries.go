package fc

import (
	"math"
	"reflect"
	"time"

	"github.com/tidwall/btree"
)

type waiterEntry struct {
	pri wakePriority
	cb  ReadinessCallback
}

type waiterEntries struct {
	entries btree.BTreeG[waiterEntry]
	byCB    map[ReadinessCallback]waiterEntry
}

func waiterEntryLess(a, b waiterEntry) bool {
	if a.pri < b.pri {
		return true
	} else if a.pri > b.pri {
		return false
	}
	// ReadinessCallbacks, as interface values, aren't comparable. To use them for equality in
	// our btree, with some arbitrary ordering, we compare the underlying pointers. This
	// depends on Go's GC not moving values, and on all clients using pointer values.
	return reflect.ValueOf(a.cb).Pointer() < reflect.ValueOf(b.cb).Pointer()
}

func newWaiterEntries() *waiterEntries {
	return &waiterEntries{
		entries: *btree.NewBTreeGOptions(
			waiterEntryLess,
			// degree 3 allows up to 5 values per node without splitting
			btree.Options{Degree: 3, NoLocks: true},
		),
		byCB: make(map[ReadinessCallback]waiterEntry),
	}
}

// clears all waiters
func (we *waiterEntries) clear() {
	we.entries.Clear()
	clear(we.byCB)
}

// returns number of waiters
func (we *waiterEntries) len() int {
	return we.entries.Len()
}

// adds a callback, or updates the wake priority of an existing callback
func (we *waiterEntries) add(cb ReadinessCallback, pri int32, age time.Time) {
	we.remove(cb)
	e := waiterEntry{
		pri: makeWakePriority(pri, age),
		cb:  cb,
	}
	we.entries.Set(e)
	we.byCB[cb] = e
}

// removes a callback
func (we *waiterEntries) remove(cb ReadinessCallback) {
	if e, ok := we.byCB[cb]; ok {
		we.entries.Delete(e)
	}
}

// returns wake priority of earliest waiter, or 0 if none
func (we *waiterEntries) minPriority() wakePriority {
	e, ok := we.entries.Min()
	if ok {
		return e.pri
	}
	return 0
}

// removes up to the first n callbacks and returns them as a slice
func (we *waiterEntries) take(n int32) (out []ReadinessCallback) {
	we.entries.DeleteAscend(
		waiterEntry{pri: math.MinInt64},
		func(e waiterEntry) btree.Action {
			if int32(len(out)) < n {
				out = append(out, e.cb)
				delete(we.byCB, e.cb)
				return btree.Delete
			}
			return btree.Stop
		},
	)
	return out
}
