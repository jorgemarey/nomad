// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

//go:build !ent
// +build !ent

package structs

import (
	"errors"
	"fmt"

	multierror "github.com/hashicorp/go-multierror"
)

func (n *Namespace) Canonicalize() {
	if n.NodePoolConfiguration == nil {
		n.NodePoolConfiguration = &NamespaceNodePoolConfiguration{}
	}
	n.NodePoolConfiguration.Canonicalize()
}

func (n *NamespaceNodePoolConfiguration) Canonicalize() {
	if n.Default == "" {
		n.Default = NodePoolDefault
	}
}

func (n *NamespaceNodePoolConfiguration) Validate() error {
	if n == nil {
		return nil
	}

	if n.Allowed != nil && n.Denied != nil {
		return errors.New("only one of 'Allowed', 'Denied' can be set")
	}

	// TODO: check for allowed to be a valid string?
	return nil
}

func (n *NamespaceVaultConfiguration) Canonicalize() {}

func (n *NamespaceVaultConfiguration) Validate() error {
	if n != nil {
		return errors.New("Multi-Cluster Vault is unlicensed.")
	}
	return nil
}

func (n *NamespaceConsulConfiguration) Canonicalize() {}

func (n *NamespaceConsulConfiguration) Validate() error {
	if n != nil {
		return errors.New("Multi-Cluster Consul is unlicensed.")
	}
	return nil
}

func (m *Multiregion) Validate(jobType string, jobDatacenters []string) error {
	// TODO
	if m != nil {
		return errors.New("Multiregion jobs are unlicensed.")
	}

	return nil
}

func (p *ScalingPolicy) validateType() multierror.Error {
	var mErr multierror.Error

	// Check policy type and target
	switch p.Type {
	case ScalingPolicyTypeHorizontal:
		targetErr := p.validateTargetHorizontal()
		mErr.Errors = append(mErr.Errors, targetErr.Errors...)
	default:
		mErr.Errors = append(mErr.Errors, fmt.Errorf(`scaling policy type "%s" is not valid`, p.Type))
	}

	return mErr
}

func (j *Job) GetEntScalingPolicies() []*ScalingPolicy {
	return nil
}
