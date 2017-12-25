// +build !pro,!ent

package scheduler

import (
	memdb "github.com/hashicorp/go-memdb"
	"github.com/hashicorp/nomad/nomad/structs"
)

// StateEnterprise are the available state store methods for the enterprise
// version.
type StateEnterprise interface {
	// NamespaceByName is used to lookup a namespace
	NamespaceByName(ws memdb.WatchSet, name string) (*structs.Namespace, error)
}
