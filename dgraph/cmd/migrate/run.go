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

	flag := Migrate.Cmd.Flags()
	flag.StringP("mysql_user", "", "",
		"The MySQL user for logging in")
	flag.StringP("mysql_password", "", "",
		"The MySQL password used for logging in")
	flag.StringP("mysql_db", "", "", "The MySQL database to import")
	flag.StringP("mysql_tables", "", "", "The MySQL tables to import, "+
		"an empty string means all tables in the database")
	flag.StringP("output_schema", "s", "", "The schema output file")
	flag.StringP("output_data", "o", "", "The data output file")
}
