// +build !pro,!ent

package nomad

import (
	memdb "github.com/hashicorp/go-memdb"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/raft"
	"github.com/ugorji/go/codec"
)

// registerLogAppliers is a no-op for open-source only FSMs.
func (n *nomadFSM) registerLogAppliers() {
	n.enterpriseAppliers[structs.NamespaceUpsertRequestType] = n.applyNamespaceUpsert
	n.enterpriseAppliers[structs.NamespaceDeleteRequestType] = n.applyNamespaceDelete
}

// registerSnapshotRestorers is a no-op for open-source only FSMs.
func (n *nomadFSM) registerSnapshotRestorers() {
	n.enterpriseRestorers[NamespaceSnapshot] = restoreNamespace
}

// persistEnterpriseTables is a no-op for open-source only FSMs.
func (s *nomadSnapshot) persistEnterpriseTables(sink raft.SnapshotSink, encoder *codec.Encoder) error {
	if err := s.persistNamespaces(sink, encoder); err != nil {
		return err
	}

	return nil
}

// persistNamespaces persists all the namespaces.
func (s *nomadSnapshot) persistNamespaces(sink raft.SnapshotSink, encoder *codec.Encoder) error {
	// Get all the namespaces
	ws := memdb.NewWatchSet()
	namespaces, err := s.snap.Namespaces(ws)
	if err != nil {
		return err
	}

	for {
		// Get the next item
		raw := namespaces.Next()
		if raw == nil {
			break
		}

		// Prepare the request struct
		namespace := raw.(*structs.Namespace)

		// Write out a namespace registration
		sink.Write([]byte{byte(NamespaceSnapshot)})
		if err := encoder.Encode(namespace); err != nil {
			return err
		}
	}
	return nil
}
