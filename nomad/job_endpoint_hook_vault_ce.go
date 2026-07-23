// Copyright IBM Corp. 2015, 2026
// SPDX-License-Identifier: BUSL-1.1

//go:build !ent

package nomad

import (
	"fmt"
	"strings"

	"github.com/hashicorp/nomad/nomad/structs"
)

// validateNamespaces returns an error if the job contains any Vault namespaces.
func (jobVaultHook) validateNamespaces(blocks map[string]map[string]*structs.Vault) error {

	requestedNamespaces := structs.VaultNamespaceSet(blocks)
	if len(requestedNamespaces) > 0 {
		return fmt.Errorf("%w, Namespaces: %s", ErrMultipleNamespaces, strings.Join(requestedNamespaces, ", "))
	}
	return nil
}

func (h jobVaultHook) validateClustersForNamespace(_ *structs.Job, blocks map[string]map[string]*structs.Vault) error {
	// TODO: here we should also check namespace configuration
	// example  (j jobNodePoolValidatingHook) enterpriseValidation

	for _, tg := range blocks {
		for _, vault := range tg {
			config := h.srv.config.VaultConfigs[vault.Cluster]
			if config == nil {
				return fmt.Errorf("vault cluster %s not found", vault.Cluster)
			}
		}
	}

	return nil
}

func (j jobVaultHook) Mutate(job *structs.Job) (*structs.Job, []error, error) {
	defaultCluster := structs.VaultDefaultCluster
	ns, err := j.srv.State().NamespaceByName(nil, job.Namespace)
	if err != nil {
		return nil, nil, err
	}
	if ns == nil {
		return nil, nil, fmt.Errorf("namespace %s not found", job.Namespace)
	}
	if ns.VaultConfiguration != nil && ns.VaultConfiguration.Default != "" {
		defaultCluster = ns.VaultConfiguration.Default
	}

	for _, tg := range job.TaskGroups {
		for _, task := range tg.Tasks {
			if task.Vault == nil || task.Vault.Cluster != "" {
				continue
			}
			task.Vault.Cluster = defaultCluster
		}
	}

	return job, nil, nil
}
