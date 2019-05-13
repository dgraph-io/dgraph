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
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	logger  = log.New(os.Stderr, "", 0)
	Migrate x.SubCommand
	quiet   bool // enabling quiet mode would suppress the warning logs
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
	flag.StringP("user", "", "", "The user for logging in")
	flag.StringP("password", "", "", "The password used for logging in")
	flag.StringP("db", "", "", "The database to import")
	flag.StringP("tables", "", "", "The comma separated list of "+
		"tables to import, an empty string means importing all tables in the database")
	flag.StringP("output_schema", "s", "schema.txt", "The schema output file")
	flag.StringP("output_data", "o", "sql.rdf", "The data output file")
	flag.StringP("separator", "p", ".", "The separator for constructing predicate names")
	flag.BoolP("quiet", "q", false, "Enable quiet mode to suppress the warning logs")
}

func run(conf *viper.Viper) error {
	user := conf.GetString("user")
	db := conf.GetString("db")
	password := conf.GetString("password")
	tables := conf.GetString("tables")
	schemaOutput := conf.GetString("output_schema")
	dataOutput := conf.GetString("output_data")
	quiet = conf.GetBool("quiet")
	separator = conf.GetString("separator")

	switch {
	case len(user) == 0:
		logger.Fatalf("The user property should not be empty.")
	case len(db) == 0:
		logger.Fatalf("The db property should not be empty.")
	case len(password) == 0:
		logger.Fatalf("The password property should not be empty.")
	case len(schemaOutput) == 0:
		logger.Fatalf("Please use the --output_schema option to " +
			"provide the schema output file.")
	case len(dataOutput) == 0:
		logger.Fatalf("Please use the --output_data option to provide the data output file.")
	}

	if err := checkFile(schemaOutput); err != nil {
		return err
	}
	if err := checkFile(dataOutput); err != nil {
		return err
	}

	initDataTypes()

	pool, err := getPool(user, db, password)
	if err != nil {
		return err
	}
	defer pool.Close()

	tablesToRead, err := showTables(pool, tables)
	if err != nil {
		return err
	}

	tableInfos := make(map[string]*sqlTable, 0)
	for _, table := range tablesToRead {
		tableInfo, err := parseTables(pool, table, db)
		if err != nil {
			return err
		}
		tableInfos[tableInfo.tableName] = tableInfo
	}
	populateReferencedByColumns(tableInfos)

	tableGuides := getTableGuides(tableInfos)

	return generateSchemaAndData(&dumpMeta{
		tableInfos:  tableInfos,
		tableGuides: tableGuides,
		sqlPool:     pool,
	}, schemaOutput, dataOutput)
}

// checkFile checks if the program is trying to output to an existing file.
// If so, we would need to ask the user whether we should overwrite the file or abort the program.
func checkFile(file string) error {
	if _, err := os.Stat(file); err == nil {
		// The file already exists.
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Printf("overwriting the file %s (y/N)? ", file)
			text, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			text = strings.TrimSpace(text)

			if len(text) == 0 || strings.ToLower(text) == "n" {
				return fmt.Errorf("not allowed to overwrite %s", file)
			}
			if strings.ToLower(text) == "y" {
				return nil
			}
			fmt.Println("Please type y or n (hit enter to choose n)")
		}
	}

	// The file does not exist.
	return nil
}

// generateSchemaAndData opens the two files schemaOutput and dataOutput,
// then it dumps schema to the writer backed by schemaOutput, and data in RDF format
// to the writer backed by dataOutput
func generateSchemaAndData(dumpMeta *dumpMeta, schemaOutput string, dataOutput string) error {
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
