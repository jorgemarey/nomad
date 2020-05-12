package sentinel

import "encoding/json"

type Policy struct {
	Name  string           // human-friendly name
	Level EnforcementLevel // enforcement level
	Code  string           // The code of the policy
}

// EnforcementLevel controls the behavior of policy execution by allowing
// optional policies, overridable policies, etc. There are three enforcement
// levels documented in the enum.
type EnforcementLevel string

const (
	// Advisory means that the policy is allowed to fail. This is reported
	// to the host system which should then log this.
	//
	// SoftMandatory is a policy that is required, but can be overidden
	// on failure. The override is specified within EvalOpts and the mechanism
	// for setting it is determined by the host system.
	//
	// HardMandatory is a policy that is required and cannot be overidden.
	// This is the default enforcement level.
	Advisory      EnforcementLevel = "advisory"
	SoftMandatory EnforcementLevel = "soft-mandatory"
	HardMandatory EnforcementLevel = "hard-mandatory"
)

// MarshalJSON implements json.Marshaler. A policy marshals only to its name.
func (p *Policy) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Name)
}
