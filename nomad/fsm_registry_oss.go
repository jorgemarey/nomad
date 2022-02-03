//go:build !ent
// +build !ent

package nomad

import (
	"fmt"
	"time"

	metrics "github.com/armon/go-metrics"
	memdb "github.com/hashicorp/go-memdb"
	"github.com/hashicorp/go-msgpack/codec"
	"github.com/hashicorp/nomad/nomad/state"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/raft"
)

// registerLogAppliers is a no-op for open-source only FSMs.
func (n *nomadFSM) registerLogAppliers() {
	n.enterpriseAppliers[structs.SentinelPolicyUpsertRequestType] = n.applySentinelPolicyUpsert
	n.enterpriseAppliers[structs.SentinelPolicyDeleteRequestType] = n.applySentinelPolicyDelete
}

// registerSnapshotRestorers is a no-op for open-source only FSMs.
func (n *nomadFSM) registerSnapshotRestorers() {
	n.enterpriseRestorers[SentinelPolicySnapshot] = restoreSentinelPolicy
}

// persistEnterpriseTables is a no-op for open-source only FSMs.
func (s *nomadSnapshot) persistEnterpriseTables(sink raft.SnapshotSink, encoder *codec.Encoder) error {
	if err := s.persistSentinelPolicies(sink, encoder); err != nil {
		sink.Cancel()
		return err
	}

	return nil
}

// persistSentinelPolicies is used to persist sentinel policies
func (s *nomadSnapshot) persistSentinelPolicies(sink raft.SnapshotSink,
	encoder *codec.Encoder) error {
	// Get all the policies
	ws := memdb.NewWatchSet()
	policies, err := s.snap.SentinelPolicies(ws)
	if err != nil {
		return err
	}

	for {
		// Get the next item
		raw := policies.Next()
		if raw == nil {
			break
		}

		// Prepare the request struct
		policy := raw.(*structs.SentinelPolicy)

		// Write out a policy registration
		sink.Write([]byte{byte(SentinelPolicySnapshot)})
		if err := encoder.Encode(policy); err != nil {
			return err
		}
	}
	return nil
}

// applySentinelPolicyUpsert is used to upsert a set of policies
func (n *nomadFSM) applySentinelPolicyUpsert(buf []byte, index uint64) interface{} {
	defer metrics.MeasureSince([]string{"nomad", "fsm", "apply_sentinel_policy_upsert"}, time.Now())
	var req structs.SentinelPolicyUpsertRequest
	if err := structs.Decode(buf, &req); err != nil {
		panic(fmt.Errorf("failed to decode request: %v", err))
	}

	if err := n.state.UpsertSentinelPolicies(index, req.Policies); err != nil {
		n.logger.Error("UpsertSentinelPolicies failed", "error", err)
		return err
	}
	return nil
}

// applySentinelPolicyDelete is used to delete a set of policies
func (n *nomadFSM) applySentinelPolicyDelete(buf []byte, index uint64) interface{} {
	defer metrics.MeasureSince([]string{"nomad", "fsm", "apply_sentinel_policy_delete"}, time.Now())
	var req structs.SentinelPolicyDeleteRequest
	if err := structs.Decode(buf, &req); err != nil {
		panic(fmt.Errorf("failed to decode request: %v", err))
	}

	if err := n.state.DeleteSentinelPolicies(index, req.Names); err != nil {
		n.logger.Error("DeleteSentinelPolicies failed", "error", err)
		return err
	}
	return nil
}

// restoreSentinelPolicy is used to restore a sentinel policy
func restoreSentinelPolicy(restore *state.StateRestore, dec *codec.Decoder) error {
	policy := new(structs.SentinelPolicy)
	if err := dec.Decode(policy); err != nil {
		return err
	}
	return restore.SentinelPolicyRestore(policy)
}
