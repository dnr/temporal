// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package membership

import (
	"fmt"
	"sync"

	"go.uber.org/fx"
	"google.golang.org/grpc/resolver"
)

const GRPCResolverScheme = "membership"

var GRPCResolverProviderModule = fx.Options(
	fx.Invoke(registerGRPCResolver),
)

func MakeGRPCResolverURL(service string) string {
	return fmt.Sprintf("%s://%s", GRPCResolverScheme, service)
}

func registerGRPCResolver(monitor Monitor) {
	resolver.Register(&grpcBuilder{monitor: monitor})
}

type grpcBuilder struct {
	monitor Monitor
}

func (m *grpcBuilder) Scheme() string {
	return GRPCResolverScheme
}

func (m *grpcBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	// See MakeGRPCResolverURL: the service ends up as the "host" of the parsed URL
	service := target.URL.Host
	r, err := m.monitor.GetResolver(service)
	if err != nil {
		return nil, err
	}
	resolver := &grpcResolver{
		cc:       cc,
		r:        r,
		notifyCh: make(chan *ChangedEvent, 1),
	}
	resolver.start()
	return resolver, nil
}

type grpcResolver struct {
	cc       resolver.ClientConn
	r        ServiceResolver
	notifyCh chan *ChangedEvent
	wg       sync.WaitGroup
}

func (m *grpcResolver) start() {
	m.r.AddListener(fmt.Sprintf("%p", m), m.notifyCh)
	m.wg.Add(1)
	go m.listen()

	// Try once to get address synchronously. If this fails, it's okay, we'll listen for
	// changes and update the resolver later.
	m.resolve()
}

func (m *grpcResolver) listen() {
	for range m.notifyCh {
		m.resolve()
	}
	m.wg.Done()
}

func (m *grpcResolver) resolve() {
	members := m.r.Members()
	if len(members) == 0 {
		// grpc considers it an error if we report no addresses, and fails the connection eagerly.
		// Instead, just wait until we're notified by membership.
		return
	}
	addresses := make([]resolver.Address, 0, len(members))
	for _, hostInfo := range members {
		addresses = append(addresses, resolver.Address{
			Addr: hostInfo.GetAddress(),
		})
	}
	m.cc.UpdateState(resolver.State{Addresses: addresses})
}

func (m *grpcResolver) ResolveNow(_ resolver.ResolveNowOptions) {
	select {
	case m.notifyCh <- nil:
	default:
	}
}

func (m *grpcResolver) Close() {
	m.r.RemoveListener(fmt.Sprintf("%p", m))
	close(m.notifyCh)
	m.wg.Wait() // wait until listen() exits
}
