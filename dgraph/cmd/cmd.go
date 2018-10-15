/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package cmd

import (
	"github.com/dgraph-io/dgraph/dgraph/cmd/bulk"
	"github.com/dgraph-io/dgraph/dgraph/cmd/cert"
	"github.com/dgraph-io/dgraph/dgraph/cmd/conv"
	"github.com/dgraph-io/dgraph/dgraph/cmd/debug"
	"github.com/dgraph-io/dgraph/dgraph/cmd/live"
	"github.com/dgraph-io/dgraph/dgraph/cmd/server"
	"github.com/dgraph-io/dgraph/dgraph/cmd/version"
	"github.com/dgraph-io/dgraph/dgraph/cmd/zero"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func bindAll(subcommands ...*x.SubCommand) {
	for _, sc := range subcommands {
		RootCmd.AddCommand(sc.Cmd)
		sc.Conf = viper.New()
		sc.Conf.BindPFlags(sc.Cmd.Flags())
		sc.Conf.BindPFlags(RootCmd.PersistentFlags())
		sc.Conf.AutomaticEnv()
		sc.Conf.SetEnvPrefix(sc.EnvPrefix)
		cobra.OnInitialize(func() {
			cfg := rootConf.GetString("config")
			if cfg == "" {
				return
			}
			sc.Conf.SetConfigFile(cfg)
			x.Check(x.Wrapf(sc.Conf.ReadInConfig(), "reading config"))
		})
	}
}

func init() {
	bindAll(&bulk.Bulk, &cert.Cert, &conv.Conv, &live.Live, &server.Server, &zero.Zero,
		&version.Version, &debug.Debug)
}
