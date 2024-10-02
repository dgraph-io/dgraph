//go:build oss
// +build oss

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package acl

import (
	"github.com/spf13/cobra"

	"github.com/dgraph-io/dgo/v240/protos/api"
	"github.com/dgraph-io/dgraph/v24/x"
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
