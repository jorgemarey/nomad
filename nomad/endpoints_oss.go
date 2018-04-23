// +build !pro,!ent

package nomad

import "net/rpc"

// EnterpriseEndpoints holds the set of custom endpoints to register
type EnterpriseEndpoints struct {
	Namespace *Namespace
}

// NewEnterpriseEndpoints returns the custom nomad endpoints
func NewEnterpriseEndpoints(s *Server) *EnterpriseEndpoints {
	return &EnterpriseEndpoints{
		Namespace: &Namespace{s},
	}
}

// Register is a no-op in oss.
func (e *EnterpriseEndpoints) Register(s *rpc.Server) {
	s.Register(e.Namespace)
}
