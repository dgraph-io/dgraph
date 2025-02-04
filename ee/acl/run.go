//go:build oss
// +build oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 */

package acl

import (
	"github.com/spf13/cobra"

	"github.com/dgraph-io/dgo/v240/protos/api"
	"github.com/hypermodeinc/dgraph/v24/x"
)

var CmdAcl x.SubCommand

func init() {
	CmdAcl.Cmd = &cobra.Command{
		Use:         "acl",
		Short:       "Enterprise feature. Not supported in oss version",
		Annotations: map[string]string{"group": "security"},
	}
	CmdAcl.Cmd.SetHelpTemplate(x.NonRootTemplate)
}

// CreateGroupNQuads cretes NQuads needed to store a group with the give ID.
func CreateGroupNQuads(groupId string) []*api.NQuad {
	return nil
}

// CreateUserNQuads creates the NQuads needed to store a user with the given ID and
// password in the ACL system.
func CreateUserNQuads(userId, password string) []*api.NQuad {
	return nil
}
