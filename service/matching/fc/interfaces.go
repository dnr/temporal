package fc

import persistencespb "go.temporal.io/server/api/persistence/v1"

type userDataManager interface {
	GetUserData() (*persistencespb.VersionedTaskQueueUserData, chan struct{}, error)
}

type rateLimitManager interface {
}

type fcTask interface {
	Limiters() *Limiters
}

type ReadinessCallback interface {
	OnReady()
}
