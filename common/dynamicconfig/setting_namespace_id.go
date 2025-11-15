package dynamicconfig

import (
	"strings"
	"time"

	"go.temporal.io/server/common/namespace"
)

type (
	TypedPropertyFnWithNamespaceIDFilter[T any]   func(id namespace.ID) T
	TypedSubscribableWithNamespaceIDFilter[T any] func(id namespace.ID, callback func(T)) (v T, cancel func())

	BoolPropertyFnWithNamespaceIDFilter     = TypedPropertyFnWithNamespaceIDFilter[bool]
	IntPropertyFnWithNamespaceIDFilter      = TypedPropertyFnWithNamespaceIDFilter[int]
	FloatPropertyFnWithNamespaceIDFilter    = TypedPropertyFnWithNamespaceIDFilter[float64]
	StringPropertyFnWithNamespaceIDFilter   = TypedPropertyFnWithNamespaceIDFilter[string]
	DurationPropertyFnWithNamespaceIDFilter = TypedPropertyFnWithNamespaceIDFilter[time.Duration]
)

// unusual prefix with charaters that can never appear in a namespace name
const nsIDAsNamePrefix = "~~ ID!"

// We can work with namespace IDs instead of names. To include an ID where a name is expected
// (e.g. in Constraints.Namespace), wrap it with this function.
func NamespaceIDAsName(id namespace.ID) string {
	return nsIDAsNamePrefix + id.String()
}

func isIDAsName(s string) bool {
	return strings.HasPrefix(s, nsIDAsNamePrefix)
}

func (s NamespaceTypedSetting[T]) GetByID(c *Collection) TypedPropertyFnWithNamespaceIDFilter[T] {
	return func(id namespace.ID) T {
		prec := []Constraints{{Namespace: NamespaceIDAsName(id)}, {}}
		return matchAndConvert(
			c,
			s.key,
			s.def,
			s.convert,
			prec,
		)
	}
}

func (s NamespaceTypedConstrainedDefaultSetting[T]) GetByID(c *Collection) TypedPropertyFnWithNamespaceIDFilter[T] {
	return func(id namespace.ID) T {
		prec := []Constraints{{Namespace: NamespaceIDAsName(id)}, {}}
		return matchAndConvertWithConstrainedDefault(
			c,
			s.key,
			s.cdef,
			s.convert,
			prec,
		)
	}
}

func (s NamespaceTypedSetting[T]) SubscribeByID(c *Collection) TypedSubscribableWithNamespaceIDFilter[T] {
	return func(id namespace.ID, callback func(T)) (T, func()) {
		prec := []Constraints{{Namespace: NamespaceIDAsName(id)}, {}}
		return subscribe(c, s.key, s.def, s.convert, prec, callback)
	}
}
