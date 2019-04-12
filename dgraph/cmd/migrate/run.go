/*
 * Copyright 2017-2019 Dgraph Labs, Inc. and Contributors
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

package migrate

import (
	"log"
	"os"

	"github.com/dgraph-io/dgraph/x"
	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"
)

var (
	logger  = log.New(os.Stderr, "", 0)
	Migrate x.SubCommand
)

func init() {
	Migrate.Cmd = &cobra.Command{
		Use:   "migrate",
		Short: "Run the Dgraph migrate tool",
		Run: func(cmd *cobra.Command, args []string) {
			if err := run(Migrate.Conf); err != nil {
				logger.Fatalf("%v\n", err)
			}
		},
	}
	Migrate.EnvPrefix = "DGRAPH_MIGRATE"

	flag := Migrate.Cmd.PersistentFlags()
	flag.StringP("mysql_user", "", "", "The MySQL user used for logging in")
	flag.StringP("mysql_password", "", "", "The MySQL password used for logging in")
	flag.StringP("mysql_db", "", "", "The MySQL database to import")
	flag.StringP("mysql_tables", "", "", "The MySQL tables to import")

	/*
		subcommands := initSubCommands()
		for _, sc := range subcommands {
			Migrate.Cmd.AddCommand(sc.Cmd)
			sc.Conf = viper.New()
			if err := sc.Conf.BindPFlags(sc.Cmd.Flags()); err != nil {
				glog.Fatalf("Unable to bind flags for command %v: %v", sc, err)
			}
			if err := sc.Conf.BindPFlags(Migrate.Cmd.PersistentFlags()); err != nil {
				glog.Fatalf("Unable to bind persistent flags from acl for command %v: %v", sc, err)
			}
			sc.Conf.SetEnvPrefix(sc.EnvPrefix)
		}


			// pass down the values in the config file to the subcommand viper configuration
			cobra.OnInitialize(func() {
				cfg := Migrate.Conf.GetString("config")
				if cfg == "" {
					return
				}
				for _, sc := range subcommands {
					sc.Conf.SetConfigFile(cfg)
					x.Check(x.Wrapf(sc.Conf.ReadInConfig(), "reading config"))
				}
			})
	*/
}

/*
func initSubCommands() []*x.SubCommand {
	var genGuideCmd x.SubCommand
	genGuideCmd.Cmd = &cobra.Command{
		Use:   "gen_guide",
		Short: "Run the gen_guide tool to generate a migration guide",
		Run: func(cmd *cobra.Command, args []string) {
			if err := run(genGuideCmd.Conf); err != nil {
				logger.Fatalf("%v\n", err)
			}
		},
	}
	genGuideFlags := genGuideCmd.Cmd.Flags()
	genGuideFlags.StringP("output", "o", "guide.json",
		"The output file for the table guide")
	//genGuideFlags.StringP("config", "", "", "The config file

	return []*x.SubCommand{&genGuideCmd}
}
*/
