// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

//go:build !ent
// +build !ent

package nomad

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/nomad/nomad/structs"
)

func (h jobConsulHook) Validate(job *structs.Job) ([]error, error) {

	requiresToken := false

	clusterNeedsToken := func(name string, identity *structs.WorkloadIdentity) bool {
		if identity != nil {
			return false
		}
		config := h.srv.config.ConsulConfigs[name]
		if config != nil {
			return !config.AllowsUnauthenticated()
		}
		return false
	}

	for _, group := range job.TaskGroups {

		groupPartition := ""

		if group.Consul != nil {
			groupPartition = group.Consul.Partition
			if err := h.validateCluster(group.Consul.Cluster); err != nil {
				return nil, err
			}
		}

		for _, service := range group.Services {
			if service.Provider == structs.ServiceProviderConsul {
				if err := h.validateCluster(service.Cluster); err != nil {
					return nil, err
				}
				requiresToken = clusterNeedsToken(
					service.Cluster, service.Identity) || requiresToken
			}
		}

		for _, task := range group.Tasks {
			for _, service := range task.Services {
				if service.Provider == structs.ServiceProviderConsul {
					if err := h.validateCluster(service.Cluster); err != nil {
						return nil, err
					}
					requiresToken = clusterNeedsToken(
						service.Cluster, service.Identity) || requiresToken
				}
			}

			if task.Consul != nil {
				err := h.validateTaskPartitionMatchesGroup(groupPartition, task.Consul)
				if err != nil {
					return nil, err
				}

				if err := h.validateCluster(task.Consul.Cluster); err != nil {
					return nil, err
				}
			}

			if len(task.Templates) == 0 {
				continue
			}

			var clusterIdentity *structs.WorkloadIdentity
			taskCluster := task.GetConsulClusterName(group)
			for _, identity := range task.Identities {
				if identity.Name == "consul_"+taskCluster {
					clusterIdentity = identity
					break
				}
			}
			requiresToken = clusterNeedsToken(
				taskCluster, clusterIdentity) || requiresToken
		}
	}

	if !requiresToken {
		return nil, nil
	}

	warnings := []error{
		errors.New("Setting a Consul token when submitting a job is deprecated and will be removed in Nomad 1.9. Migrate your Consul configuration to use workload identity"),
	}

	// helper function that checks if the Consul token supplied with the job has
	// sufficient ACL permissions for:
	//   - registering services into namespace of each group
	//   - reading kv store of each group
	//   - establishing consul connect services
	checkConsulToken := func(usages map[string]*structs.ConsulUsage) error {
		ctx := context.Background()
		for namespace, usage := range usages {
			if err := h.srv.consulACLs.CheckPermissions(ctx, namespace, usage, job.ConsulToken); err != nil {
				return fmt.Errorf("job-submitter consul token denied: %w", err)
			}
		}
		return nil
	}

	// Enforce the job-submitter has a Consul token with necessary ACL permissions.
	if err := checkConsulToken(job.ConsulUsages()); err != nil {
		return warnings, err
	}
	return warnings, nil
}

func (h jobConsulHook) validateCluster(name string) error {
	// TODO: here we should also check namespace configuration
	// example  (j jobNodePoolValidatingHook) enterpriseValidation

	config := h.srv.config.ConsulConfigs[name]
	if config == nil {
		return fmt.Errorf("consul cluster %s not found", name)
	}
	return nil
}

// Mutate ensures that the job's Consul cluster has been configured to be the
// default Consul cluster if unset
func (j jobConsulHook) Mutate(job *structs.Job) (*structs.Job, []error, error) {
	defaultCluster := structs.ConsulDefaultCluster
	ns, err := j.srv.State().NamespaceByName(nil, job.Namespace)
	if err != nil {
		return nil, nil, err
	}
	if ns == nil {
		return nil, nil, fmt.Errorf("namespace %s not found", job.Namespace)
	}
	if ns.ConsulConfiguration != nil && ns.ConsulConfiguration.Default != "" {
		defaultCluster = ns.ConsulConfiguration.Default
	}
	return j.mutateImpl(job, defaultCluster), nil, nil
}
