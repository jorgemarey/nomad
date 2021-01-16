// +build !ent

package state

import (
	"fmt"

	memdb "github.com/hashicorp/go-memdb"
	"github.com/hashicorp/nomad/nomad/structs"
)

// NamespacesByQuota is used to lookup namespaces by quota
func (s *StateStore) NamespacesByQuota(ws memdb.WatchSet, quota string) (memdb.ResultIterator, error) {
	txn := s.db.ReadTxn()
	return s.namespacesByQuotaImpl(ws, txn, quota)
}

// namespacesByQuotaImpl is used to lookup namespaces by quota
func (s *StateStore) namespacesByQuotaImpl(ws memdb.WatchSet, txn *txn, quota string) (memdb.ResultIterator, error) {
	iter, err := txn.Get(TableNamespaces, "quota", quota)
	if err != nil {
		return nil, fmt.Errorf("namespaces lookup failed: %v", err)
	}
	ws.Add(iter.WatchCh())

	return iter, nil
}

// UpsertSentinelPolicies is used to create or update a set of Sentinel policies
func (s *StateStore) UpsertSentinelPolicies(index uint64, policies []*structs.SentinelPolicy) error {
	txn := s.db.WriteTxn(index)
	defer txn.Abort()

	for _, policy := range policies {
		// Ensure the policy hash is non-nil. This should be done outside the state store
		// for performance reasons, but we check here for defense in depth.
		if len(policy.Hash) == 0 {
			policy.SetHash()
		}

		// Check if the policy already exists
		existing, err := txn.First(TableSentinelPolicies, "id", policy.Name)
		if err != nil {
			return fmt.Errorf("policy lookup failed: %v", err)
		}

		// Update all the indexes
		if existing != nil {
			policy.CreateIndex = existing.(*structs.SentinelPolicy).CreateIndex
			policy.ModifyIndex = index
		} else {
			policy.CreateIndex = index
			policy.ModifyIndex = index
		}

		// Update the policy
		if err := txn.Insert(TableSentinelPolicies, policy); err != nil {
			return fmt.Errorf("upserting policy failed: %v", err)
		}
	}

	// Update the indexes table
	if err := txn.Insert("index", &IndexEntry{TableSentinelPolicies, index}); err != nil {
		return fmt.Errorf("index update failed: %v", err)
	}

	return txn.Commit()
}

// DeleteSentinelPolicies deletes the policies with the given names
func (s *StateStore) DeleteSentinelPolicies(index uint64, names []string) error {
	txn := s.db.WriteTxn(index)
	defer txn.Abort()

	// Delete the policy
	for _, name := range names {
		if _, err := txn.DeleteAll(TableSentinelPolicies, "id", name); err != nil {
			return fmt.Errorf("deleting sentinel policy failed: %v", err)
		}
	}
	if err := txn.Insert("index", &IndexEntry{TableSentinelPolicies, index}); err != nil {
		return fmt.Errorf("index update failed: %v", err)
	}
	return txn.Commit()
}

// SentinelPolicyByName is used to lookup a policy by name
func (s *StateStore) SentinelPolicyByName(ws memdb.WatchSet, name string) (*structs.SentinelPolicy, error) {
	txn := s.db.ReadTxn()

	watchCh, existing, err := txn.FirstWatch(TableSentinelPolicies, "id", name)
	if err != nil {
		return nil, fmt.Errorf("sentinel policy lookup failed: %v", err)
	}
	ws.Add(watchCh)

	if existing != nil {
		return existing.(*structs.SentinelPolicy), nil
	}
	return nil, nil
}

// SentinelPolicyByNamePrefix is used to lookup policies by prefix
func (s *StateStore) SentinelPolicyByNamePrefix(ws memdb.WatchSet, prefix string) (memdb.ResultIterator, error) {
	txn := s.db.ReadTxn()

	iter, err := txn.Get(TableSentinelPolicies, "id_prefix", prefix)
	if err != nil {
		return nil, fmt.Errorf("sentinel policy lookup failed: %v", err)
	}
	ws.Add(iter.WatchCh())

	return iter, nil
}

// SentinelPolicies returns an iterator over all the sentinel policies
func (s *StateStore) SentinelPolicies(ws memdb.WatchSet) (memdb.ResultIterator, error) {
	txn := s.db.ReadTxn()

	// Walk the entire table
	iter, err := txn.Get(TableSentinelPolicies, "id")
	if err != nil {
		return nil, err
	}
	ws.Add(iter.WatchCh())
	return iter, nil
}

// SentinelPoliciesByScope returns an iterator over all the sentinel policies by scope
func (s *StateStore) SentinelPoliciesByScope(ws memdb.WatchSet, scope string) (memdb.ResultIterator, error) {
	txn := s.db.ReadTxn()

	// Walk the entire table
	iter, err := txn.Get(TableSentinelPolicies, "scope", scope)
	if err != nil {
		return nil, err
	}
	ws.Add(iter.WatchCh())
	return iter, nil
}

// SentinelPolicyRestore is used to restore an Sentinel policy
func (r *StateRestore) SentinelPolicyRestore(policy *structs.SentinelPolicy) error {
	if err := r.txn.Insert(TableSentinelPolicies, policy); err != nil {
		return fmt.Errorf("inserting sentinel policy failed: %v", err)
	}
	return nil
}

// quotaSpecExists on returns whether the quota exists
func (s *StateStore) quotaSpecExists(txn *txn, name string) (bool, error) {
	return false, nil
}

func (s *StateStore) quotaReconcile(index uint64, txn *txn, newQuota, oldQuota string) error {
	return nil
}

// updateEntWithAlloc is used to update Nomad Enterprise objects when an allocation is
// added/modified/deleted
func (s *StateStore) updateEntWithAlloc(index uint64, new, existing *structs.Allocation, txn *txn) error {
	return nil
}

// deleteRecommendationsByJob deletes all recommendations for the specified job
func (s *StateStore) deleteRecommendationsByJob(index uint64, txn Txn, job *structs.Job) error {
	return nil
}

// updateJobRecommendations updates/deletes job recommendations as necessary for a job update
func (s *StateStore) updateJobRecommendations(index uint64, txn Txn, prevJob, newJob *structs.Job) error {
	return nil
}
