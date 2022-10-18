//go:build !ent
// +build !ent

package nomad

import (
	autopilot "github.com/hashicorp/raft-autopilot"
	improvedAutopilot "github.com/jorgemarey/autopilot"

	"github.com/hashicorp/nomad/nomad/structs"
)

func (s *Server) autopilotPromoter() autopilot.Promoter {
	return improvedAutopilot.New(improvedAutopilot.WithLogger(s.logger))
}

// autopilotServerExt returns the autopilot-enterprise.Server extensions needed
// for ENT feature support, but this is the empty OSS implementation.
func (s *Server) autopilotServerExt(parts *serverParts) interface{} {
	return improvedAutopilot.ExtraServerInfo{
		NonVoter: parts.NonVoter,
	}
}

// autopilotConfigExt returns the autopilot-enterprise.Config extensions needed
// for ENT feature support, but this is the empty OSS implementation.
func autopilotConfigExt(c *structs.AutopilotConfig) interface{} {
	return improvedAutopilot.ExtraConfig{
		RedundancyZoneTag:       AutopilotRZTag,
		UpgradeVersionTag:       AutopilotVersionTag,
		DisableUpgradeMigration: c.DisableUpgradeMigration,
	}
}
