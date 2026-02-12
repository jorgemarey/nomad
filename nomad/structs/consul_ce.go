// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: BUSL-1.1

//go:build !ent
// +build !ent

package structs

func (c *Consul) GetNamespace() string {
	if c != nil && c.Namespace != "" {
		return c.Namespace // TODO(meigas): should we return default?
	}
	return ""
}

// GetConsulClusterName gets the Consul cluster for this task. Only a single
// default cluster is supported in Nomad CE.
func (t *Task) GetConsulClusterName(tg *TaskGroup) string {
	cluster := ConsulDefaultCluster
	if tg.Consul != nil && tg.Consul.Cluster != "" {
		cluster = tg.Consul.Cluster
	}
	if t.Consul != nil && t.Consul.Cluster != "" {
		cluster = t.Consul.Cluster
	}
	return cluster
}

// GetConsulClusterName gets the Consul cluster for this service. Only a single
// default cluster is supported in Nomad CE.
func (s *Service) GetConsulClusterName(tg *TaskGroup) string {
	cluster := ConsulDefaultCluster
	if tg.Consul != nil && tg.Consul.Cluster != "" {
		cluster = tg.Consul.Cluster
	}
	if s.Cluster != "" {
		cluster = s.Cluster
	}
	return cluster
}
