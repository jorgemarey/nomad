package nomad

import (
	"context"
	"fmt"

	metrics "github.com/armon/go-metrics"
	"github.com/hashicorp/consul/agent/consul/autopilot"
	"github.com/hashicorp/raft"
	"github.com/hashicorp/serf/serf"
)

const (
	// AutopilotRZTag is the Serf tag to use for the redundancy zone value
	// when passing the server metadata to Autopilot.
	AutopilotRZTag = "ap_zone"

	// AutopilotRZTag is the Serf tag to use for the custom version value
	// when passing the server metadata to Autopilot.
	AutopilotVersionTag = "ap_version"
)

// AutopilotDelegate is a Nomad delegate for autopilot operations.
type AutopilotDelegate struct {
	server *Server
}

func (d *AutopilotDelegate) AutopilotConfig() *autopilot.Config {
	c := d.server.getOrCreateAutopilotConfig()
	if c == nil {
		return nil
	}

	conf := &autopilot.Config{
		CleanupDeadServers:      c.CleanupDeadServers,
		LastContactThreshold:    c.LastContactThreshold,
		MaxTrailingLogs:         c.MaxTrailingLogs,
		ServerStabilizationTime: c.ServerStabilizationTime,
		DisableUpgradeMigration: c.DisableUpgradeMigration,
		ModifyIndex:             c.ModifyIndex,
		CreateIndex:             c.CreateIndex,
	}

	if c.EnableRedundancyZones {
		conf.RedundancyZoneTag = AutopilotRZTag
	}
	if c.EnableCustomUpgrades {
		conf.UpgradeVersionTag = AutopilotVersionTag
	}

	return conf
}

func (d *AutopilotDelegate) FetchStats(ctx context.Context, servers []serf.Member) map[string]*autopilot.ServerStats {
	return d.server.statsFetcher.Fetch(ctx, servers)
}

func (d *AutopilotDelegate) IsServer(m serf.Member) (*autopilot.ServerInfo, error) {
	ok, parts := isNomadServer(m)
	if !ok || parts.Region != d.server.Region() {
		return nil, nil
	}

	server := &autopilot.ServerInfo{
		Name:   m.Name,
		ID:     parts.ID,
		Addr:   parts.Addr,
		Build:  parts.Build,
		Status: m.Status,
	}
	return server, nil
}

// NotifyHealth heartbeats a metric for monitoring if we're the leader.
func (d *AutopilotDelegate) NotifyHealth(health autopilot.OperatorHealthReply) {
	if d.server.raft.State() == raft.Leader {
		metrics.SetGauge([]string{"nomad", "autopilot", "failure_tolerance"}, float32(health.FailureTolerance))
		if health.Healthy {
			metrics.SetGauge([]string{"nomad", "autopilot", "healthy"}, 1)
		} else {
			metrics.SetGauge([]string{"nomad", "autopilot", "healthy"}, 0)
		}
	}
}

func (d *AutopilotDelegate) PromoteNonVoters(conf *autopilot.Config, health autopilot.OperatorHealthReply) ([]raft.Server, error) {
	future := d.server.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		return nil, fmt.Errorf("failed to get raft configuration: %v", err)
	}
	servers := future.Configuration().Servers

	// Find any non-voters eligible for promotion.
	stable := autopilot.PromoteStableServers(conf, health, servers)

	// Remove non voting servers
	promoted := d.filterNonVoting(stable)

	// if no servers to add just return now
	if len(promoted) == 0 {
		return promoted, nil
	}

	// Filter by zone
	if conf.RedundancyZoneTag != "" {
		promoted = d.filterZoneServers(promoted, servers)
	}

	return promoted, nil
}

func (d *AutopilotDelegate) Raft() *raft.Raft {
	return d.server.raft
}

func (d *AutopilotDelegate) Serf() *serf.Serf {
	return d.server.serf
}

func (d *AutopilotDelegate) filterNonVoting(stable []raft.Server) []raft.Server {
	var promoted []raft.Server
	for _, server := range stable {
		part, ok := d.server.localPeers[server.Address]
		if !ok || !part.NonVoter {
			promoted = append(promoted, server)
		}
	}
	return promoted
}

// zone returns the zone of a server and if it's ok
func (d *AutopilotDelegate) zone(server raft.Server) (string, bool) {
	var zone string
	for _, member := range d.Serf().Members() {
		if server.ID == raft.ServerID(member.Tags["id"]) {
			return member.Tags[AutopilotRZTag], member.Status != serf.StatusFailed
		}
	}
	return zone, true
}

func (d *AutopilotDelegate) filterZoneServers(initial []raft.Server, servers []raft.Server) []raft.Server {
	zoneVoter := make(map[string]bool)
	for _, server := range servers { // we set if there're a voter en every zone we know
		if zone, ok := d.zone(server); zone != "" {
			zoneVoter[zone] = zoneVoter[zone] || (autopilot.IsPotentialVoter(server.Suffrage) && ok)
		}
	}
	promoted := make([]raft.Server, 0)
	zones := make(map[string][]raft.Server)
	for _, server := range initial {
		zone, _ := d.zone(server)
		if zone == "" { // If server has no zone we add it
			promoted = append(promoted, server)
		} else {
			if zoneVoter[zone] {
				continue
			}
			if _, ok := zones[zone]; !ok {
				zones[zone] = make([]raft.Server, 0)
			}
			zones[zone] = append(zones[zone], server)
		}
	}
	// We iterate over the zones that don't have any voter
	for _, zs := range zones {
		promoted = append(promoted, zs[0]) // we pick one, in this case the first
	}
	return promoted
}
