// +build !pro,!ent

package nomad

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
func (e *EnterpriseEndpoints) Register(s *Server) {
	s.rpcServer.Register(e.Namespace)
}
