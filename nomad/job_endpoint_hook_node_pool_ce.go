// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

//go:build !ent
// +build !ent

package nomad

import (
	"fmt"

	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/ryanuber/go-glob"
)

// enterpriseValidation implements any admission hooks for node pools for Nomad
// Enterprise.
func (j jobNodePoolValidatingHook) enterpriseValidation(job *structs.Job, pool *structs.NodePool) ([]error, error) {
	ns, err := j.srv.State().NamespaceByName(nil, job.Namespace)
	if err != nil {
		return nil, err
	}

	// By default, all node pools are allowed
	if ns.NodePoolConfiguration == nil {
		return nil, nil
	}

	// If an empty list is provided only the namespace's default node pool is allowed
	if allowed := ns.NodePoolConfiguration.Allowed; allowed != nil {
		if pool.Name == ns.NodePoolConfiguration.Default {
			return nil, nil
		}
		for _, np := range allowed {
			if glob.Glob(np, pool.Name) {
				return nil, nil
			}
		}
		return nil, fmt.Errorf("node pool '%s' is not allowed", pool.Name)
	}

	if denied := ns.NodePoolConfiguration.Denied; denied != nil {
		for _, np := range denied {
			if glob.Glob(np, pool.Name) {
				return nil, fmt.Errorf("node pool '%s' is not allowed", pool.Name)
			}
		}
	}
	return nil, nil
}

// jobNodePoolMutatingHook mutates the job on Nomad Enterprise only.
type jobNodePoolMutatingHook struct {
	srv *Server
}

func (c jobNodePoolMutatingHook) Name() string {
	return "node-pool-mutation"
}

func (c jobNodePoolMutatingHook) Mutate(job *structs.Job) (*structs.Job, []error, error) {
	if job.NodePool != "" {
		return job, nil, nil
	}

	ns, err := c.srv.State().NamespaceByName(nil, job.Namespace)
	if err != nil {
		return nil, nil, err
	}
	if ns.NodePoolConfiguration != nil {
		job.NodePool = ns.NodePoolConfiguration.Default
	}

	if job.NodePool == "" {
		job.NodePool = structs.NodePoolDefault
	}

	return job, nil, nil
}
