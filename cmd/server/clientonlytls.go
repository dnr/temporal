package main

import (
	"crypto/tls"
	"time"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/rpc/encryption"
)

type clientOnlyTLS struct {
	logger log.Logger
	base   encryption.TLSConfigProvider
}

func NewClientOnlyTLS(
	logger log.Logger,
	base encryption.TLSConfigProvider,
) encryption.TLSConfigProvider {
	return &clientOnlyTLS{logger, base}
}

func (c *clientOnlyTLS) GetInternodeServerConfig() (*tls.Config, error) {
	// hack to not enable TLS on internode server !!
	c.logger.Info("getting internode server config but returning nil!")

	return nil, nil

	// return c.base.GetInternodeServerConfig()
}

func (c *clientOnlyTLS) GetInternodeClientConfig() (*tls.Config, error) {
	return c.base.GetInternodeClientConfig()
}

func (c *clientOnlyTLS) GetFrontendServerConfig() (*tls.Config, error) {
	return c.base.GetFrontendServerConfig()
}

func (c *clientOnlyTLS) GetFrontendClientConfig() (*tls.Config, error) {
	return c.base.GetFrontendClientConfig()
}

func (c *clientOnlyTLS) GetRemoteClusterClientConfig(hostname string) (*tls.Config, error) {
	return c.base.GetRemoteClusterClientConfig(hostname)
}

func (c *clientOnlyTLS) GetExpiringCerts(timeWindow time.Duration) (expiring encryption.CertExpirationMap, expired encryption.CertExpirationMap, err error) {
	return c.base.GetExpiringCerts(timeWindow)
}
