// +build !pro,!ent

package nomad

import (
	"github.com/hashicorp/consul/agent/consul/autopilot"
	improvedAutopilot "github.com/jorgemarey/autopilot"

	"github.com/jorgemarey/sentinel"
)

type EnterpriseState struct{
	sentinel *sentinel.Sentinel
}

func (s *Server) setupEnterprise(config *Config) error {
	// Set up the OSS version of autopilot
	apDelegate := improvedAutopilot.New(s.logger, &AutopilotDelegate{s})
	s.autopilot = autopilot.NewAutopilot(s.logger, apDelegate, config.AutopilotInterval, config.ServerHealthInterval)

	s.sentinel = sentinel.New(nil)
	return nil
}

func (s *Server) startEnterpriseBackground() {}
