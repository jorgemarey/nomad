// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package nomad

import (
	"fmt"

	"github.com/hashicorp/nomad/nomad/structs"
)

// jobVaultHook is an job registration admission controller for Vault blocks.
type jobVaultHook struct {
	srv *Server
}

func (jobVaultHook) Name() string {
	return "vault"
}

func (h jobVaultHook) Validate(job *structs.Job) ([]error, error) {
	vaultBlocks := job.Vault()
	if len(vaultBlocks) == 0 {
		return nil, nil
	}

	for _, tg := range vaultBlocks {
		for _, vaultBlock := range tg {
			vconf := h.srv.config.VaultConfigs[vaultBlock.Cluster]
			if !vconf.IsEnabled() {
				return nil, fmt.Errorf("Vault %q not enabled but used in the job",
					vaultBlock.Cluster)
			}
		}
	}

	// Check namespaces.
	if err := h.validateNamespaces(vaultBlocks); err != nil {
		return nil, err
	}

	return nil, h.validateClustersForNamespace(job, vaultBlocks)
}

func hasWid(job *structs.Job, groupName, taskName string, vaultBlock *structs.Vault) bool {
	for _, group := range job.TaskGroups {
		if group.Name != groupName {
			continue
		}
		for _, task := range group.Tasks {
			if task.Name != taskName {
				continue
			}
			for _, wid := range task.Identities {
				if wid.Name == vaultBlock.IdentityName() {
					return true
				}
			}
		}
	}
	return false
}
