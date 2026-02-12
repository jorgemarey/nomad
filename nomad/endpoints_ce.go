// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: BUSL-1.1

//go:build !ent
// +build !ent

package nomad

import "net/rpc"

// EnterpriseEndpoints holds the set of custom endpoints to register
type EnterpriseEndpoints struct {
	Sentinel *Sentinel
}

// NewEnterpriseEndpoints returns a stub of the enterprise endpoints since there
// are none in oss
func NewEnterpriseEndpoints(s *Server, ctx *RPCContext) *EnterpriseEndpoints {
	return &EnterpriseEndpoints{
		Sentinel: &Sentinel{srv: s, ctx: ctx, logger: s.logger.Named("sentinel")},
	}
}

// Register is a no-op in oss.
func (e *EnterpriseEndpoints) Register(s *rpc.Server) {
	s.Register(e.Sentinel)
}
