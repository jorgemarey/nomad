// +build !ent

package state

import (
	"fmt"

	memdb "github.com/hashicorp/go-memdb"
	"github.com/hashicorp/nomad/nomad/structs"
)

// enterpriseInit is used to initialize the state store with enterprise
// objects.
func (s *StateStore) enterpriseInit() error {
	// Create the default namespace. This is safe to do every time we create the
	// state store. There are two main cases, a brand new cluster in which case
	// each server will have the same default namespace object, or a new cluster
	// in which case if the default namespace has been modified, it will be
	// overridden by the restore code path.
	defaultNs := &structs.Namespace{
		Name:        structs.DefaultNamespace,
		Description: structs.DefaultNamespaceDescription,
	}

	if err := s.UpsertNamespaces(1, []*structs.Namespace{defaultNs}); err != nil {
		return fmt.Errorf("inserting default namespace failed: %v", err)
	}

	return nil
}

// namespaceExists returns whether a namespace exists
func (s *StateStore) namespaceExists(txn *memdb.Txn, namespace string) (bool, error) {
	if namespace == structs.DefaultNamespace {
		return true, nil
	}

	existing, err := txn.First(TableNamespace, "id", namespace)
	if err != nil {
		return false, fmt.Errorf("namespace lookup failed: %v", err)
	}

	return existing != nil, nil
}

// NamespaceByName is used to lookup a namespace by name
func (s *StateStore) NamespaceByName(ws memdb.WatchSet, name string) (*structs.Namespace, error) {
	txn := s.db.Txn(false)
	return s.namespaceByNameImpl(ws, txn, name)
}

// namespaceByNameImpl is used to lookup a namespace by name
func (s *StateStore) namespaceByNameImpl(ws memdb.WatchSet, txn *memdb.Txn, name string) (*structs.Namespace, error) {
	watchCh, existing, err := txn.FirstWatch(TableNamespace, "id", name)
	if err != nil {
		return nil, fmt.Errorf("namespace lookup failed: %v", err)
	}
	ws.Add(watchCh)

	if existing != nil {
		return existing.(*structs.Namespace), nil
	}
	return nil, nil
}

// NamespacesByNamePrefix is used to lookup namespaces by prefix
func (s *StateStore) NamespacesByNamePrefix(ws memdb.WatchSet, namePrefix string) (memdb.ResultIterator, error) {
	txn := s.db.Txn(false)

	iter, err := txn.Get(TableNamespace, "id_prefix", namePrefix)
	if err != nil {
		return nil, fmt.Errorf("namespaces lookup failed: %v", err)
	}
	ws.Add(iter.WatchCh())

	return iter, nil
}

// UpsertNamespaces is used to register or update a set of namespaces
func (s *StateStore) UpsertNamespaces(index uint64, namespaces []*structs.Namespace) error {
	txn := s.db.Txn(true)
	defer txn.Abort()

	for _, ns := range namespaces {
		if err := s.upsertNamespaceImpl(index, txn, ns); err != nil {
			return err
		}
	}

	if err := txn.Insert("index", &IndexEntry{TableNamespace, index}); err != nil {
		return fmt.Errorf("index update failed: %v", err)
	}

	txn.Commit()
	return nil
}

// upsertNamespaceImpl is used to upsert a namespace
func (s *StateStore) upsertNamespaceImpl(index uint64, txn *memdb.Txn, namespace *structs.Namespace) error {
	// Ensure the namespace hash is non-nil. This should be done outside the state store
	// for performance reasons, but we check here for defense in depth.
	ns := namespace
	if len(ns.Hash) == 0 {
		ns.SetHash()
	}

	// Check if the namespace already exists
	existing, err := txn.First(TableNamespace, "id", ns.Name)
	if err != nil {
		return fmt.Errorf("namespace lookup failed: %v", err)
	}

	// Setup the indexes correctly
	if existing != nil {
		exist := existing.(*structs.Namespace)
		ns.CreateIndex = exist.CreateIndex
	} else {
		ns.CreateIndex = index
	}
	ns.ModifyIndex = index

	// Validate that the quota exists
	if ns.Quota != "" {
		return fmt.Errorf("Namespaces can only attach quotas in Nomad Premium")
	}

	// Insert the namespace
	if err := txn.Insert(TableNamespace, ns); err != nil {
		return fmt.Errorf("namespace insert failed: %v", err)
	}

	return nil
}

// DeleteNamespaces is used to remove a set of namespaces
func (s *StateStore) DeleteNamespaces(index uint64, names []string) error {
	txn := s.db.Txn(true)
	defer txn.Abort()

	for _, name := range names {
		// Lookup the namespace
		existing, err := txn.First(TableNamespace, "id", name)
		if err != nil {
			return fmt.Errorf("namespace lookup failed: %v", err)
		}
		if existing == nil {
			return fmt.Errorf("namespace not found")
		}

		ns := existing.(*structs.Namespace)
		if ns.Name == structs.DefaultNamespace {
			return fmt.Errorf("default namespace can not be deleted")
		}

		// Ensure that the namespace doesn't have any non-terminal jobs
		iter, err := s.jobsByNamespaceImpl(nil, name, txn)
		if err != nil {
			return err
		}

		for {
			raw := iter.Next()
			if raw == nil {
				break
			}
			job := raw.(*structs.Job)

			if job.Status != structs.JobStatusDead {
				return fmt.Errorf("namespace %q contains at least one non-terminal job %q. "+
					"All jobs must be terminal in namespace before it can be deleted", name, job.ID)
			}
		}

		// Delete the namespace
		if err := txn.Delete(TableNamespace, existing); err != nil {
			return fmt.Errorf("namespace deletion failed: %v", err)
		}
	}

	if err := txn.Insert("index", &IndexEntry{TableNamespace, index}); err != nil {
		return fmt.Errorf("index update failed: %v", err)
	}

	txn.Commit()
	return nil
}

// Namespaces returns an iterator over all the namespaces
func (s *StateStore) Namespaces(ws memdb.WatchSet) (memdb.ResultIterator, error) {
	txn := s.db.Txn(false)

	// Walk the entire namespace table
	iter, err := txn.Get(TableNamespace, "id")
	if err != nil {
		return nil, fmt.Errorf("namespace lookup failed: %v", err)
	}
	ws.Add(iter.WatchCh())
	return iter, nil
}

// NamespacesByQuota is used to lookup namespaces by quota
func (s *StateStore) NamespacesByQuota(ws memdb.WatchSet, quota string) (memdb.ResultIterator, error) {
	txn := s.db.Txn(false)
	return s.namespacesByQuotaImpl(ws, txn, quota)
}

// namespacesByQuotaImpl is used to lookup namespaces by quota
func (s *StateStore) namespacesByQuotaImpl(ws memdb.WatchSet, txn *memdb.Txn, quota string) (memdb.ResultIterator, error) {
	iter, err := txn.Get(TableNamespace, "quota", quota)
	if err != nil {
		return nil, fmt.Errorf("namespaces lookup failed: %v", err)
	}
	ws.Add(iter.WatchCh())

	return iter, nil
}

// NamespaceRestore is used to restore a namespace
func (r *StateRestore) NamespaceRestore(ns *structs.Namespace) error {
	if err := r.txn.Insert(TableNamespace, ns); err != nil {
		return fmt.Errorf("namespace insert failed: %v", err)
	}
	return nil
}

// UpsertSentinelPolicies is used to create or update a set of Sentinel policies
func (s *StateStore) UpsertSentinelPolicies(index uint64, policies []*structs.SentinelPolicy) error {
	txn := s.db.Txn(true)
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

	txn.Commit()
	return nil
}

// DeleteSentinelPolicies deletes the policies with the given names
func (s *StateStore) DeleteSentinelPolicies(index uint64, names []string) error {
	txn := s.db.Txn(true)
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
	txn.Commit()
	return nil
}

// SentinelPolicyByName is used to lookup a policy by name
func (s *StateStore) SentinelPolicyByName(ws memdb.WatchSet, name string) (*structs.SentinelPolicy, error) {
	txn := s.db.Txn(false)

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
	txn := s.db.Txn(false)

	iter, err := txn.Get(TableSentinelPolicies, "id_prefix", prefix)
	if err != nil {
		return nil, fmt.Errorf("sentinel policy lookup failed: %v", err)
	}
	ws.Add(iter.WatchCh())

	return iter, nil
}

// SentinelPolicies returns an iterator over all the sentinel policies
func (s *StateStore) SentinelPolicies(ws memdb.WatchSet) (memdb.ResultIterator, error) {
	txn := s.db.Txn(false)

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
	txn := s.db.Txn(false)

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

// updateEntWithAlloc is used to update Nomad Enterprise objects when an allocation is
// added/modified/deleted
func (s *StateStore) updateEntWithAlloc(index uint64, new, existing *structs.Allocation, txn *memdb.Txn) error {
	return nil
}
