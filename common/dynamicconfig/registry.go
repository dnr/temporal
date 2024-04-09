package dynamicconfig

import (
	"fmt"
	"strings"
)

type (
	registry struct {
		settings map[string]GenericSetting
		queried  bool
	}
)

var (
	globalRegistry registry
)

// Packages should call Register or RegisterGeneric on all known settings from init or static
// initializers.
func RegisterGeneric(s GenericSetting) {
	if globalRegistry.queried {
		panic("must call Register from init()")
	}
	if globalRegistry.settings == nil {
		globalRegistry.settings = make(map[string]GenericSetting)
	}
	keyStr := strings.ToLower(s.GetKey().String())
	if globalRegistry.settings[keyStr] != nil {
		panic(fmt.Sprintf("duplicate registration of dynamic config key: %q", keyStr))
	}
	globalRegistry.settings[keyStr] = s
}

func Register[S GenericSetting](s S) S {
	RegisterGeneric(s)
	return s
}

func (r *registry) query(k Key) GenericSetting {
	if !globalRegistry.queried {
		globalRegistry.queried = true
	}
	return globalRegistry.settings[strings.ToLower(k.String())]
}
