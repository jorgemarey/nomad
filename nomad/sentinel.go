package nomad

import (
	"errors"
	"fmt"
	"time"

	metrics "github.com/armon/go-metrics"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/jorgemarey/sentinel"
)

// sentinelDataCallback materializes the Sentinel data
type sentinelDataCallback func() map[string]interface{}

func (s *Server) enforceScope(override bool, scope string, dataCB sentinelDataCallback) (warn, err error) {
	// Fast-path if ACLs are disabled
	if !s.config.ACLEnabled {
		return nil, nil
	}

	// Gather the applicable policies
	registered, err := s.sentinelPoliciesByScope(scope)
	if err != nil {
		return nil, err
	}

	defer metrics.MeasureSinceWithLabels([]string{"nomad", "sentinel", "enforce_scope"}, time.Now(),
		[]metrics.Label{{Name: "scope", Value: scope}})

	// Prepare the policies for execution
	prepared, err := prepareSentinelPolicies(s.sentinel, registered)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare policies: %v", err)
	}

	// Materialize the data if we have a callback
	var data map[string]interface{}
	if dataCB != nil {
		data = dataCB()
	}

	// Evaluate the policy
	result := s.sentinel.Eval(prepared, &sentinel.EvalOpts{
		Data:     data,
		Override: override,
	})

	// Convert the result into warnings or errors
	return sentinelResultToWarnErr(result)
}

// sentinelPoliciesByScope returns all the applicable policies by scope
func (s *Server) sentinelPoliciesByScope(scope string) ([]*structs.SentinelPolicy, error) {
	// Snapshot the current state
	snap, err := s.State().Snapshot()
	if err != nil {
		return nil, err
	}

	// Gather the applicable policies
	iter, err := snap.SentinelPoliciesByScope(nil, scope)
	if err != nil {
		return nil, err
	}
	var registered []*structs.SentinelPolicy
	for {
		raw := iter.Next()
		if raw == nil {
			break
		}
		registered = append(registered, raw.(*structs.SentinelPolicy))
	}
	return registered, nil
}

// prepareSentinelPolicies converts all the raw policies into compiled
// policies. The caller must unlock all the policies when complete.
func prepareSentinelPolicies(sent *sentinel.Sentinel, policies []*structs.SentinelPolicy) ([]*sentinel.Policy, error) {
	// Convert the policies to sentinel policies
	var out []*sentinel.Policy
	for _, inp := range policies {
		p := &sentinel.Policy{
			Name: inp.Name,
			Level: sentinel.EnforcementLevel(inp.EnforcementLevel),
			Code: inp.Policy,
		}

		out = append(out, p)
	}
	return out, nil
}

// sentinelResultToWarnErr is used to convert a sentinel evaluation result
// into either a set of warnings or a set of errors.
func sentinelResultToWarnErr(result *sentinel.EvalResult) (warn, err error) {
	// Check for an error
	if result.Error != nil {
		return nil, errors.New(result.String())
	}

	// Collect all the warnings / errors
	var mWarn multierror.Error
	var mErr multierror.Error
	for _, policyResult := range result.Policies {
		if !policyResult.Result {
			msg := fmt.Errorf("%s : %s", policyResult.Policy.Name,
				policyResult.String())
			if policyResult.AllowedFailure {
				mWarn.Errors = append(mWarn.Errors, msg)
			} else {
				mErr.Errors = append(mErr.Errors, msg)
			}
		}
	}
	return mWarn.ErrorOrNil(), mErr.ErrorOrNil()
}