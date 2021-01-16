// +build !ent

package nomad

import (
	"bytes"
	"context"
	"time"

	"golang.org/x/time/rate"

	"github.com/hashicorp/nomad/nomad/state"
	"github.com/hashicorp/nomad/nomad/structs"
)

// establishEnterpriseLeadership is a no-op on OSS.
func (s *Server) establishEnterpriseLeadership(stopCh chan struct{}) error {
	// Start replication of Sentinel Policies if we are not the authoritative region.
	if s.config.Region != s.config.AuthoritativeRegion {
		if s.config.ACLEnabled {
			go s.replicateSentinelPolicies(stopCh)
		}
	}

	return nil
}

// revokeEnterpriseLeadership is a no-op on OSS>
func (s *Server) revokeEnterpriseLeadership() error {
	return nil
}

// replicateSentinelPolicies is used to replicate Sentinel policies from
// the authoritative region to this region.
func (s *Server) replicateSentinelPolicies(stopCh chan struct{}) {
	req := structs.SentinelPolicyListRequest{
		QueryOptions: structs.QueryOptions{
			Region:     s.config.AuthoritativeRegion,
			AllowStale: true,
		},
	}
	limiter := rate.NewLimiter(replicationRateLimit, int(replicationRateLimit))
	s.logger.Debug("starting Sentinel policy replication from authoritative region", "authoritative_region", req.Region)

START:
	for {
		select {
		case <-stopCh:
			return
		default:
		}

		// Rate limit how often we attempt replication
		limiter.Wait(context.Background())

		// Fetch the list of policies
		var resp structs.SentinelPolicyListResponse
		req.AuthToken = s.ReplicationToken()
		err := s.forwardRegion(s.config.AuthoritativeRegion,
			"Sentinel.ListPolicies", &req, &resp)
		if err != nil {
			s.logger.Error("failed to fetch policies from authoritative region", "error", err)
			goto ERR_WAIT
		}

		// Perform a two-way diff
		delete, update := diffSentinelPolicies(s.State(), req.MinQueryIndex, resp.Policies)

		// Delete policies that should not exist
		if len(delete) > 0 {
			args := &structs.SentinelPolicyDeleteRequest{
				Names: delete,
			}
			_, _, err := s.raftApply(structs.SentinelPolicyDeleteRequestType, args)
			if err != nil {
				s.logger.Error("failed to delete policies", "error", err)
				goto ERR_WAIT
			}
		}

		// Fetch any outdated policies
		var fetched []*structs.SentinelPolicy
		if len(update) > 0 {
			req := structs.SentinelPolicySetRequest{
				Names: update,
				QueryOptions: structs.QueryOptions{
					Region:        s.config.AuthoritativeRegion,
					AuthToken:     s.ReplicationToken(),
					AllowStale:    true,
					MinQueryIndex: resp.Index - 1,
				},
			}
			var reply structs.SentinelPolicySetResponse
			if err := s.forwardRegion(s.config.AuthoritativeRegion,
				"Sentinel.GetPolicies", &req, &reply); err != nil {
				s.logger.Error("failed to fetch policies from authoritative region", "error", err)
				goto ERR_WAIT
			}
			for _, policy := range reply.Policies {
				fetched = append(fetched, policy)
			}
		}

		// Update local policies
		if len(fetched) > 0 {
			args := &structs.SentinelPolicyUpsertRequest{
				Policies: fetched,
			}
			_, _, err := s.raftApply(structs.SentinelPolicyUpsertRequestType, args)
			if err != nil {
				s.logger.Error("failed to update policies", "error", err)
				goto ERR_WAIT
			}
		}

		// Update the minimum query index, blocks until there
		// is a change.
		req.MinQueryIndex = resp.Index
	}

ERR_WAIT:
	select {
	case <-time.After(s.config.ReplicationBackoff):
		goto START
	case <-stopCh:
		return
	}
}

// diffSentinelPolicies is used to perform a two-way diff between the local
// policies and the remote policies to determine which policies need to
// be deleted or updated.
func diffSentinelPolicies(state *state.StateStore, minIndex uint64, remoteList []*structs.SentinelPolicyListStub) (delete []string, update []string) {
	// Construct a set of the local and remote policies
	local := make(map[string][]byte)
	remote := make(map[string]struct{})

	// Add all the local policies
	iter, err := state.SentinelPolicies(nil)
	if err != nil {
		panic("failed to iterate local policies")
	}
	for {
		raw := iter.Next()
		if raw == nil {
			break
		}
		policy := raw.(*structs.SentinelPolicy)
		local[policy.Name] = policy.Hash
	}

	// Iterate over the remote policies
	for _, rp := range remoteList {
		remote[rp.Name] = struct{}{}

		// Check if the policy is missing locally
		if localHash, ok := local[rp.Name]; !ok {
			update = append(update, rp.Name)

			// Check if policy is newer remotely and there is a hash mis-match.
		} else if rp.ModifyIndex > minIndex && !bytes.Equal(localHash, rp.Hash) {
			update = append(update, rp.Name)
		}
	}

	// Check if policy should be deleted
	for lp := range local {
		if _, ok := remote[lp]; !ok {
			delete = append(delete, lp)
		}
	}
	return
}
