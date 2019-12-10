// +build !pro,!ent

package nomad

import (
	"github.com/hashicorp/consul/agent/consul/autopilot"
	log "github.com/hashicorp/go-hclog"
	improvedAutopilot "github.com/jorgemarey/autopilot"
)

type EnterpriseState struct{}

func (s *Server) setupEnterprise(config *Config) error {
	// Set up the OSS version of autopilot
	apDelegate := improvedAutopilot.New(s.logger.StandardLoggerIntercept(&log.StandardLoggerOptions{InferLevels: true}), &AutopilotDelegate{s})
	s.autopilot = autopilot.NewAutopilot(s.logger.StandardLoggerIntercept(&log.StandardLoggerOptions{InferLevels: true}), apDelegate, config.AutopilotInterval, config.ServerHealthInterval)

	return nil
}

func (s *Server) startEnterpriseBackground() {}
