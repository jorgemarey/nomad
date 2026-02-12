// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: BUSL-1.1

//go:build !ent
// +build !ent

package nomad

import (
	"github.com/hashicorp/nomad/nomad/structs"
)

func (n *NodePool) validateLicense(pool *structs.NodePool) error {
	return nil
}
