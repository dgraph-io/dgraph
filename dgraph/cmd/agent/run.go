/*
 * Copyright 2019-2020 Dgraph Labs, Inc. and Contributors
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

package agent

import (
	"os"
	"strings"

	"github.com/dgraph-io/dgraph/dgraph/cmd/agent/debuginfo"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Agent is the subcommand for dgraph agent.
var Agent x.SubCommand

func init() {
	Agent.Cmd = &cobra.Command{
		Use:   "agent",
		Short: "Run the Dgraph agent tool",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			if err != nil {
				os.Exit(1)
			}
		},
	}

	Agent.EnvPrefix = "DGRAPH_AGENT"
	sc := debuginfo.DebugInfo
	Agent.Cmd.AddCommand(sc.Cmd)

	sc.Conf = viper.New()
	x.Check(sc.Conf.BindPFlags(sc.Cmd.Flags()))
	sc.Conf.AutomaticEnv()
	sc.Conf.SetEnvPrefix(sc.EnvPrefix)
	sc.Conf.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
}
