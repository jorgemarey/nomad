// +build !ent

package nomad

import "net/rpc"

// EnterpriseEndpoints holds the set of custom endpoints to register
type EnterpriseEndpoints struct {
	Sentinel *Sentinel
}

// NewEnterpriseEndpoints returns the custom nomad endpoints
func NewEnterpriseEndpoints(s *Server) *EnterpriseEndpoints {
	return &EnterpriseEndpoints{
		Sentinel: &Sentinel{s},
	}
}

// Register is a no-op in oss.
func (e *EnterpriseEndpoints) Register(s *rpc.Server) {
	s.Register(e.Sentinel)
}
