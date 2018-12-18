// +build oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

var CmdAcl x.SubCommand

func init() {
	CmdAcl.Cmd = &cobra.Command{
		Use:   "acl",
		Short: "Enterprise feature. Not supported in oss version",
	}
}
