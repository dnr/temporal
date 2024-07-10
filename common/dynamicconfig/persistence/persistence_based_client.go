// The MIT License
//
// Copyright (c) 2024 Temporal Technologies Inc.  All rights reserved.
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

package dynamicconfig

import (
	"context"
	"errors"
	"time"

	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/persistence"
)

type (
	PersistenceBasedClientConfig struct {
		PollInterval time.Duration `yaml:"pollInterval"`
	}

	persistenceReader struct {
		mgr persistence.ClusterMetadataManager
	}
)

func NewPersistenceBasedClient(config *PersistenceBasedClientConfig, mgr persistence.ClusterMetadataManager, logger log.Logger, doneCh <-chan interface{}) (dynamicconfig.Client, error) {
	if config == nil {
		return nil, errors.New("configuration for dynamic config client is nil")
	}
	reader := &persistenceReader{mgr: mgr}
	fbConfig := dynamicconfig.FileBasedClientConfig{
		PollInterval: config.PollInterval,
	}
	return dynamicconfig.NewFileBasedClientWithReader(reader, &fbConfig, logger, doneCh)
}

func (p *persistenceReader) GetModTime() (time.Time, error) {
	ctx := context.Background()
	res, err := p.mgr.GetDynamicConfig(ctx, &persistence.GetDynamicConfigRequest{})
	if err != nil {
		return time.Time{}, err
	}
	return res.Modified, nil
}

func (p *persistenceReader) ReadFile() ([]byte, error) {
	ctx := context.Background()
	res, err := p.mgr.GetDynamicConfig(ctx, &persistence.GetDynamicConfigRequest{
		Contents: true,
	})
	if err != nil {
		return nil, err
	}
	return res.Contents, nil
}
