// FIXME: copyright

package shard

import (
	"go.uber.org/fx"

	"go.temporal.io/server/api/historyservice/v1"
	"go.temporal.io/server/client"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/archiver"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/cluster"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/membership"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/persistence"
	"go.temporal.io/server/common/persistence/serialization"
	"go.temporal.io/server/common/resource"
	"go.temporal.io/server/common/searchattribute"
	"go.temporal.io/server/service/history/configs"
)

var Module = fx.Options(
	fx.Provide(ShardControllerProvider),
)

type ShardControllerDeps struct {
	fx.In

	Config                      *configs.Config
	Logger                      log.Logger
	ThrottledLogger             resource.ThrottledLogger
	PersistenceExecutionManager persistence.ExecutionManager
	PersistenceShardManager     persistence.ShardManager
	ClientBean                  client.Bean
	HistoryClient               historyservice.HistoryServiceClient
	HistoryServiceResolver      membership.ServiceResolver
	MetricsClient               metrics.Client
	PayloadSerializer           serialization.Serializer
	TimeSource                  clock.TimeSource
	NamespaceRegistry           namespace.Registry
	SaProvider                  searchattribute.Provider
	SaMapper                    searchattribute.Mapper
	ClusterMetadata             cluster.Metadata
	ArchivalMetadata            archiver.ArchivalMetadata
	HostInfoProvider            resource.HostInfoProvider
}

func ShardControllerProvider(d ShardControllerDeps) *ControllerImpl {
	return &ControllerImpl{
		d:                  d,
		status:             common.DaemonStatusInitialized,
		membershipUpdateCh: make(chan *membership.ChangedEvent, 10),
		shutdownCh:         make(chan struct{}),
		metricsScope:       d.MetricsClient.Scope(metrics.HistoryShardControllerScope),
		historyShards:      make(map[int32]*ContextImpl),
	}
}
