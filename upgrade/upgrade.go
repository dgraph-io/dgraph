/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package upgrade

import (
	"fmt"
	"os"

	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

var (
	// Upgrade is the sub-command used to upgrade dgraph cluster.
	Upgrade x.SubCommand
)

func init() {
	Upgrade.Cmd = &cobra.Command{
		Use:   "upgrade",
		Short: "Run the Dgraph upgrade tool",
		Run: func(cmd *cobra.Command, args []string) {
			run()
		},
	}

	flag := Upgrade.Cmd.Flags()
	flag.Bool("acl", false, "upgrade ACL from v1.2.2 to >v20.03.0")
	flag.StringP("alpha", "a", "127.0.0.1:9080", "Dgraph Alpha gRPC server address")
	flag.StringP("user", "u", "", "Username if login is required.")
	flag.StringP("password", "p", "", "Password of the user.")
	flag.BoolP("deleteOld", "d", true, "Delete the older ACL predicates")
}

func run() {
	if !Upgrade.Conf.GetBool("acl") {
		fmt.Fprint(os.Stderr, "Error! we only support acl upgrade as of now.\n")
		return
	}

	if err := upgradeACLRules(); err != nil {
		fmt.Fprintln(os.Stderr, "Error in upgrading ACL!", err)
	}
}
