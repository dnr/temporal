package dynamicconfig

import (
	enumspb "go.temporal.io/api/enums/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
)

type (
	// Client is a source of dynamic configuration. The default Client, fileBasedClient, reads
	// from a file in the filesystem, and refreshes it periodically. You can extend the server
	// with an alternate Client using ServerOptions.
	Client interface {
		// GetValue returns a set of values and associated constraints for a key. Not all
		// constraints are valid for all keys.
		//
		// ConstrainedValues are searched from more specific to less specific, so the order of
		// ConstrainedValues usually doesn't matter. The only cases where it does matter are
		// when multiple ConstrainedValues have the exact same Constraints. In that case, the
		// first one will be chosen.
		//
		// That's true even if the ConstrainedValues have different EffectiveAtTimes: the first
		// value that matches both Constraints and EffectiveAtTime will be chosen, even if a
		// later one has a _later_ EffectiveAtTime. For the most sensible semantics, then,
		// ConstrainedValues should be sorted in decreasing order of EffectiveAtTime.
		//
		// If none of the ConstrainedValues match the constraints being used for the key, then
		// the server default value will be used.
		//
		// Note that GetValue is called very often! You should not synchronously call out to an
		// external system. Instead you should keep a set of all configured values, refresh it
		// periodically or when notified, and only do in-memory lookups inside of GetValue.
		//
		// Implementations should prefer to return the same slice in response to the same key
		// as long as the value hasn't changed. Value conversions are cached using weak
		// pointers into the returned slice, so new slices will result in unnecessary calls to
		// conversion functions.
		GetValue(key Key) []ConstrainedValue
	}

	// NotifyingClient is an optional interface that a Client can also implement, that adds
	// support for faster notifications of dynamic config changes.
	NotifyingClient interface {
		// Adds a subscription to all updates from this Client. `update` will be called on any
		// change to the current value set. The caller should call `cancel` to cancel the
		// subscription. Calls to `update` will not be made concurrently.
		Subscribe(update ClientUpdateFunc) (cancel func())
	}

	// Called with modified keys on any change to the current value set.
	// Deleted keys/constraints will get a nil value.
	ClientUpdateFunc func(map[Key][]ConstrainedValue)

	// Key is a key/property stored in dynamic config. For convenience, it is recommended that
	// you treat keys as case-insensitive.
	Key string

	// ConstrainedValue is a value plus associated constraints.
	//
	// The type of the Value field depends on the key. Acceptable types will be one of:
	//   int, float64, bool, string, map[string]any, time.Duration
	//
	// If time.Duration is expected, a string is also accepted, which will be converted using
	// timestamp.ParseDurationDefaultDays. If float64 is expected, int is also accepted. In
	// other cases, the exact type must be used. If a Value is returned with an unexpected
	// type, it will be ignored.
	ConstrainedValue struct {
		Constraints Constraints
		// EffectiveAtTime is an additional constraint: time.Now().Unix() >= EffectiveAtTime
		// It can be used to help synchronize dynamic config changes closer in time across
		// nodes. It's an additional "constraint", but is not included in Constraints so that
		// that can easily be compared for equality.
		EffectiveAtTime int64 // unix seconds
		Value           any
	}
	TypedConstrainedValue[T any] struct {
		Constraints Constraints
		// We don't need EffectiveAtTime here because TypedConstrainedValue are only used for
		// defaults, where time-based constraints wouldn't make sense.
		Value T
	}

	// Constraints describe under what conditions a ConstrainedValue should be used.
	// There are few standard "constraint precedence orders" that the server uses:
	//   global precedence:
	//     no constraints
	//   namespace precedence:
	//     Namespace
	//     no constraints
	//   task queue precedence
	//     Namespace+TaskQueueName+TaskQueueType
	//     Namespace+TaskQueueName
	//     TaskQueueName
	//     Namespace
	//     no constraints
	//   shard id precedence:
	//     ShardID
	//     no constraints
	// In each case, the constraints that the server is checking and the constraints that apply
	// to the value must match exactly, including the fields that are not set (zero values).
	// That is, for keys that use namespace precedence, you must either return a
	// ConstrainedValue with only Namespace set, or with no fields set. (Or return one of
	// each.) If you return a ConstrainedValue with Namespace and ShardID set, for example,
	// that value will never be used, even if the Namespace matches.
	Constraints struct {
		Namespace     string
		NamespaceID   string
		TaskQueueName string
		TaskQueueType enumspb.TaskQueueType
		ShardID       int32
		TaskType      enumsspb.TaskType
		Destination   string
	}
)

func (k Key) String() string {
	return string(k)
}

func (cv ConstrainedValue) matches(c Constraints, now int64) bool {
	return cv.Constraints == c && (cv.EffectiveAtTime == 0 || cv.EffectiveAtTime >= now)
}
