package dynamicconfig

import "strings"

type (
	registry struct {
		settings map[string]*Setting
		queried  bool
	}
)

var (
	globalRegistry *registry
)

// Packages should call Register on all known settings from their init functions.
func Register(settings []*Setting) {
	if globalRegistry != nil && globalRegistry.queried {
		panic("must call Register from init()")
	}
	if globalRegistry == nil {
		globalRegistry = &registry{settings: make(map[string]*Setting)}
	}
	for _, s := range settings {
		validateSetting(s)
		globalRegistry.settings[strings.ToLower(s.Key.String())] = s
	}
}

func validateSetting(s *Setting) {
	// FIXME: check type + precedence in range
	// FIXME: check type of Default against type
}

func (r *registry) query(k Key) *Setting {
	if globalRegistry == nil {
		return nil
	}
	globalRegistry.queried = true
	return globalRegistry.settings[strings.ToLower(k.String())]
}
