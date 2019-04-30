/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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
	"fmt"
	"log"
	"os"

	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	flag.StringP("mysql_tables", "", "", "The comma separated list of "+
		"MySQL tables to import, an empty string means importing all tables in the database")
	flag.StringP("output_schema", "s", "", "The schema output file")
	flag.StringP("output_data", "o", "", "The data output file")
}

func run(conf *viper.Viper) error {
	mysqlUser := conf.GetString("mysql_user")
	mysqlDB := conf.GetString("mysql_db")
	mysqlPassword := conf.GetString("mysql_password")
	mysqlTables := conf.GetString("mysql_tables")
	schemaOutput := conf.GetString("output_schema")
	dataOutput := conf.GetString("output_data")

	if len(mysqlUser) == 0 {
		logger.Fatalf("the mysql_user property should not be empty")
	}
	if len(mysqlDB) == 0 {
		logger.Fatalf("the mysql_db property should not be empty")
	}
	if len(mysqlPassword) == 0 {
		logger.Fatalf("the mysql_password property should not be empty")
	}
	if len(schemaOutput) == 0 {
		logger.Fatalf("please use the --output_schema option to " +
			"provide the schema output file")
	}
	if len(dataOutput) == 0 {
		logger.Fatalf("please use the --output_data option to provide the data output file")
	}

	initDataTypes()

	pool, err := getMySQLPool(mysqlUser, mysqlDB, mysqlPassword)
	if err != nil {
		return err
	}
	defer pool.Close()

	tablesToRead, err := readMySqlTables(pool, mysqlTables)
	if err != nil {
		return err
	}

	tableInfos := make(map[string]*TableInfo, 0)
	for _, table := range tablesToRead {
		tableInfo, err := getTableInfo(table, mysqlDB, pool)
		if err != nil {
			return err
		}
		tableInfos[tableInfo.tableName] = tableInfo
	}
	populateReferencedByColumns(tableInfos)

	tableGuides := getTableGuides(tableInfos)

	return generateSchemaAndData(&DumpMeta{
		tableInfos:  tableInfos,
		tableGuides: tableGuides,
		sqlPool:     pool,
	}, schemaOutput, dataOutput)
}

// generateSchemaAndData opens the two files schemaOutput and dataOutput,
// then it dumps schema to the writer backed by schemaOutput, and data in RDF format
// to the writer backed by dataOutput
func generateSchemaAndData(dumpMeta *DumpMeta, schemaOutput string, dataOutput string) error {
	schemaWriter, schemaCancelFunc, err := getFileWriter(schemaOutput)
	if err != nil {
		return err
	}
	defer schemaCancelFunc()
	dataWriter, dataCancelFunc, err := getFileWriter(dataOutput)
	if err != nil {
		return err
	}
	defer dataCancelFunc()

	dumpMeta.dataWriter = dataWriter
	dumpMeta.schemaWriter = schemaWriter

	if err := dumpMeta.dumpSchema(); err != nil {
		return fmt.Errorf("error while writing schema file: %v", err)
	}
	if err := dumpMeta.dumpTables(); err != nil {
		return fmt.Errorf("error while writing data file: %v", err)
	}
	return nil
}
