// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

//go:build !ent
// +build !ent

package agent

import (
	hclog "github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/nomad/structs/config"
)

// EnterpriseAgent holds information and methods for enterprise functionality
// in OSS it is an empty struct.
type EnterpriseAgent struct{}

func (a *Agent) setupEnterpriseAgent(log hclog.Logger) error {
	eventer, err := newEventerAuditor(a.config.Audit, log)
	if err != nil {
		return err
	}
	a.auditor = eventer

	return nil
}

// currently this doesn't work unless we change the logLevel or something like that.
// See Agent.ShouldReload method
func (a *Agent) entReloadEventer(cfg *config.AuditConfig) error {
	var previous bool
	if enabled := a.config.Audit.Enabled; enabled != nil {
		previous = *enabled
	}

	var current bool
	if enabled := cfg.Enabled; enabled != nil {
		current = *enabled
	}

	auditor := a.auditor.(*eventerAuditor)
	if previous != current {
		if current {
			if err := auditor.configureBroker(cfg); err != nil {
				return err
			}
		}
		auditor.SetEnabled(current)
	}
	// both are equal, only do somthing if enabled
	// TODO: check deepequal to not change if nothing changes?
	if current {
		auditor.SetEnabled(false)
		if err := auditor.configureBroker(cfg); err != nil {
			return err
		}
		auditor.SetEnabled(true)
	}
	return nil
}
