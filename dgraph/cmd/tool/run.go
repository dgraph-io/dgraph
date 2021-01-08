/*Copyright 2017-2021 Dgraph Labs, Inc. and Contributors
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

package tool

import (
	"strings"

	tool "github.com/dgraph-io/dgraph/dgraph/cmd/tool/decrypt"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Tool x.SubCommand

var subcommands = []*x.SubCommand{
	&tool.Decrypt,
}

func init() {
	Tool.Cmd = &cobra.Command{
		Use:   "tool",
		Short: "Run a Dgraph tool",
		Long:  "A suite of Dgraph tools",
	}
	Tool.EnvPrefix = "DGRAPH_TOOL"
	for _, sc := range subcommands {
		Tool.Cmd.AddCommand(sc.Cmd)
		sc.Conf = viper.New()
		x.Check(sc.Conf.BindPFlags(sc.Cmd.Flags()))
		x.Check(sc.Conf.BindPFlags(Tool.Cmd.PersistentFlags()))
		sc.Conf.AutomaticEnv()
		sc.Conf.SetEnvPrefix(sc.EnvPrefix)
		// Options that contain a "." should use "_" in its place when provided as an
		// environment variable.
		sc.Conf.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	}
}
