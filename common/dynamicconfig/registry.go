package dynamicconfig

import "strings"

type (
	registry struct {
		settings map[string]GenericSetting
		queried  bool
	}
)

var (
	globalRegistry *registry
)

// Packages should call Register on all known settings from their init functions.
func Register(settings []GenericSetting) {
	if globalRegistry != nil && globalRegistry.queried {
		panic("must call Register from init()")
	}
	if globalRegistry == nil {
		globalRegistry = &registry{settings: make(map[string]GenericSetting)}
	}
	for _, s := range settings {
		validateSetting(s)
		globalRegistry.settings[strings.ToLower(s.GetKey().String())] = s
	}
}

func validateSetting(s GenericSetting) {
	// FIXME: check type + precedence in range
	// FIXME: check type of Default against type
}

func (r *registry) query(k Key) GenericSetting {
	if globalRegistry == nil {
		return nil
	}
	globalRegistry.queried = true
	return globalRegistry.settings[strings.ToLower(k.String())]
}
