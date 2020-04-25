// +build !ent

package nomad

import (
	"fmt"

	memdb "github.com/hashicorp/go-memdb"
	"github.com/hashicorp/nomad/acl"
	"github.com/hashicorp/nomad/nomad/state"
	"github.com/hashicorp/nomad/nomad/structs"
)

var (
	// allContexts are the available contexts which are searched to find matches
	// for a given prefix
	allContexts = append(ossContexts, structs.Namespaces)
)

// contextToIndex returns the index name to lookup in the state store.
func contextToIndex(ctx structs.Context) string {
	switch ctx {
	default:
		return string(ctx)
	}
}

// getEnterpriseMatch is a no-op in oss since there are no enterprise objects.
func getEnterpriseMatch(match interface{}) (id string, ok bool) {
	switch match.(type) {
	case *structs.Namespace:
		return match.(*structs.Namespace).Name, true
	default:
		return "", false
	}
}

// getEnterpriseResourceIter is used to retrieve an iterator over an enterprise
// only table.
func getEnterpriseResourceIter(context structs.Context, aclObj *acl.ACL, namespace, prefix string, ws memdb.WatchSet, state *state.StateStore) (memdb.ResultIterator, error) {
	switch context {
	case structs.Namespaces:
		iter, err := state.NamespacesByNamePrefix(ws, prefix)
		if err != nil {
			return nil, err
		}
		if aclObj == nil {
			return iter, nil
		}
		return memdb.NewFilterIterator(iter, namespaceFilter(aclObj)), nil
	default:
		return nil, fmt.Errorf("context must be one of %v or 'all' for all contexts; got %q", allContexts, context)
	}
}

// namespaceFilter wraps a namespace iterator with a filter for removing
// namespaces the ACL can't access.
func namespaceFilter(aclObj *acl.ACL) memdb.FilterFunc {
	return func(v interface{}) bool {
		return !aclObj.AllowNamespace(v.(*structs.Namespace).Name)
	}
}

// anySearchPerms returns true if the provided ACL has access to any
// capabilities required for prefix searching. Returns true if aclObj is nil.
func anySearchPerms(aclObj *acl.ACL, namespace string, context structs.Context) bool {
	if aclObj == nil {
		return true
	}

	nodeRead := aclObj.AllowNodeRead()
	allowNS := aclObj.AllowNamespace(namespace)
	jobRead := aclObj.AllowNsOp(namespace, acl.NamespaceCapabilityReadJob)
	allowVolume := acl.NamespaceValidator(acl.NamespaceCapabilityCSIListVolume,
		acl.NamespaceCapabilityCSIReadVolume,
		acl.NamespaceCapabilityListJobs,
		acl.NamespaceCapabilityReadJob)
	volRead := allowVolume(aclObj, namespace)

	if !nodeRead && !allowNS && !jobRead && !volRead { // TODO: revise this (added allowNS)
		return false
	}

	// Reject requests that explicitly specify a disallowed context. This
	// should give the user better feedback then simply filtering out all
	// results and returning an empty list.
	if !nodeRead && context == structs.Nodes {
		return false
	}
	if !allowNS && context == structs.Namespaces {
		return false
	}
	if !jobRead {
		switch context {
		case structs.Allocs, structs.Deployments, structs.Evals, structs.Jobs:
			return false
		}
	}
	if !volRead && context == structs.Volumes {
		return false
	}

	return true
}

// searchContexts returns the contexts the aclObj is valid for. If aclObj is
// nil all contexts are returned.
func searchContexts(aclObj *acl.ACL, namespace string, context structs.Context) []structs.Context {
	var all []structs.Context

	switch context {
	case structs.All:
		all = make([]structs.Context, len(allContexts))
		copy(all, allContexts)
	default:
		all = []structs.Context{context}
	}

	// If ACLs aren't enabled return all contexts
	if aclObj == nil {
		return all
	}

	jobRead := aclObj.AllowNsOp(namespace, acl.NamespaceCapabilityReadJob)
	allowVolume := acl.NamespaceValidator(acl.NamespaceCapabilityCSIListVolume,
		acl.NamespaceCapabilityCSIReadVolume,
		acl.NamespaceCapabilityListJobs,
		acl.NamespaceCapabilityReadJob)
	volRead := allowVolume(aclObj, namespace)

	// Filter contexts down to those the ACL grants access to
	available := make([]structs.Context, 0, len(all))
	for _, c := range all {
		switch c {
		case structs.Allocs, structs.Jobs, structs.Evals, structs.Deployments:
			if jobRead {
				available = append(available, c)
			}
		case structs.Namespaces:
			if aclObj.AllowNamespace(namespace) {
				available = append(available, c)
			}
		case structs.Nodes:
			if aclObj.AllowNodeRead() {
				available = append(available, c)
			}
		case structs.Volumes:
			if volRead {
				available = append(available, c)
			}
		}
	}
	return available
}
