// +build !ent

package nomad

import (
	"github.com/hashicorp/consul/agent/consul/autopilot"
	improvedAutopilot "github.com/jorgemarey/autopilot"

	"github.com/jorgemarey/sentinel"
)

// LicenseConfig allows for tunable licensing config
// primarily used for enterprise testing
type LicenseConfig struct {
	AdditionalPubKeys []string
}

type EnterpriseState struct {
	sentinel *sentinel.Sentinel
}

func (es *EnterpriseState) Features() uint64 {
	return 0
}

func (es *EnterpriseState) ReloadLicense(_ *Config) error {
	return nil
}

func (s *Server) setupEnterprise(config *Config) error {
	// Set up the OSS version of autopilot
	apDelegate := improvedAutopilot.New(s.logger, &AutopilotDelegate{s})
	s.autopilot = autopilot.NewAutopilot(s.logger, apDelegate, config.AutopilotInterval, config.ServerHealthInterval)

	s.sentinel = sentinel.New(nil)
	return nil
}
func (s *Server) startEnterpriseBackground() {}

func (s *Server) entVaultDelegate() *VaultNoopDelegate {
	return &VaultNoopDelegate{}
}
