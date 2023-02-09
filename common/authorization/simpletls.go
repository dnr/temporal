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

package authorization

import (
	"fmt"
	"strings"

	"go.temporal.io/server/common/log"
)

type simpleTLSClaimMapper struct {
	logger log.Logger
}

func NewSimpleTLSClaimMapper(logger log.Logger) ClaimMapper {
	return &simpleTLSClaimMapper{}
}

var _ ClaimMapper = (*simpleTLSClaimMapper)(nil)

func (a *simpleTLSClaimMapper) GetClaims(authInfo *AuthInfo) (*Claims, error) {
	const testClientDomain = "client.contoso.com"
	const testAdminCN = "internode.cluster-x.contoso.com"
	claims := Claims{}
	fmt.Printf("@@@ GETCLAIMS tls %#v\n", authInfo.TLSSubject)
	if authInfo.TLSSubject != nil {
		cn := authInfo.TLSSubject.CommonName
		if cn == testAdminCN {
			claims.System = RoleAdmin
		} else if ns, domain, ok := strings.Cut(cn, "."); ok && domain == testClientDomain {
			claims.Namespaces = map[string]Role{ns: RoleWriter}
		}
	}
	return &claims, nil
}
