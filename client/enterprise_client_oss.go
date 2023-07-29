//go:build !ent
// +build !ent

package client

import (
	hclog "github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/nomad/structs"
)

// EnterpriseClient holds information and methods for enterprise functionality
type EnterpriseClient struct{}

func newEnterpriseClient(logger hclog.Logger) *EnterpriseClient {
	return &EnterpriseClient{}
}

// SetFeatures is used for enterprise builds to configure enterprise features
func (ec *EnterpriseClient) SetFeatures(features uint64) {}

func (c *Client) ResolveSecretToken(secretID string) (*structs.ACLToken, error) {
	_, t, err := c.resolveTokenAndACL(secretID)
	if err != nil {
		return nil, err
	}
	return t.ACLToken, nil
}
