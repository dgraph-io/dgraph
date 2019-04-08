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

package migrate

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	_ "github.com/go-sql-driver/mysql"
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
			defer x.StartProfile(Migrate.Conf).Stop()
			if err := run(Migrate.Conf); err != nil {
				logger.Fatalf("Error while migrating: %v", err)
			}
		},
	}
	Migrate.EnvPrefix = "DGRAPH_MIGRATE"

	flag := Migrate.Cmd.Flags()
	flag.StringP("mysql_user", "", "", "The MySQL user used for logging in")
	flag.StringP("mysql_password", "", "", "The MySQL password used for logging in")
	flag.StringP("mysql_db", "", "", "The MySQL database to import")
	flag.StringP("mysql_tables", "", "", "The MySQL tables to import")
}

type CancelFunc func()

func getMySQLPool(mysqlUser string, mysqlDB string, password string) (*sql.DB, CancelFunc,
	error) {
	pool, err := sql.Open("mysql", fmt.Sprintf("%s:%s@/%s", mysqlUser, password, mysqlDB))
	if err != nil {
		return nil, nil, err
	}
	cancelFunc := func() {
		pool.Close()
	}
	return pool, cancelFunc, nil
}

// getMySqlTables will return a slice of table names using one of the following logic
// 1) if the parameter mysqlTables is not empty, this function will return a slice of table names
// by splitting the parameter with the separate comma
// 2) if the parameter is empty, this function will read all the tables under the given
// database from MySQL and then return the result

func getMySqlTables(mysqlTables string, pool *sql.DB) ([]string, error) {
	if len(mysqlTables) > 0 {
		return strings.Split(mysqlTables, ","), nil
	}
	query := "show tables"
	rows, err := pool.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := make([]string, 0)
	for rows.Next() {
		var table string
		rows.Scan(&table)
		tables = append(tables, table)
	}

	return tables, nil
}

// dumpTable reads data from a table and sends to standard output
func dumpTable(table string, pool *sql.DB) error {
	query := fmt.Sprintf(`select * from %s`, table)
	rows, err := pool.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var columns []string
	var columnTypes []*sql.ColumnType
	for rows.Next() {
		if columns == nil {
			columns, err = rows.Columns()
			if err != nil {
				return err
			}
			fmt.Printf("columns: %s", strings.Join(columns, "   "))

			columnTypes, err = rows.ColumnTypes()
			if err != nil {
				return err
			}
		}

		colValues := make([]interface{}, 0, len(columns))
		for i := 0; i < len(columns); i++ {
			switch columnTypes[i].ScanType().Kind() {
			case reflect.String:
				colValues = append(colValues, new(string))
			case reflect.Int:
				colValues = append(colValues, new(int))
			case reflect.Int32:
				colValues = append(colValues, new(int32))
			default:
				panic(fmt.Sprintf("unknown type: %v", columnTypes[i].ScanType()))
			}
		}
		rows.Scan(colValues...)

		for i := 0; i < len(columns); i++ {
			// dereference the pointer
			fmt.Printf("%v ", reflect.ValueOf(colValues[i]).Elem())
		}
		fmt.Println()
	}
	return nil
}

func run(conf *viper.Viper) error {
	mysqlUser := conf.GetString("mysql_user")
	mysqlDB := conf.GetString("mysql_db")
	mysqlPassword := conf.GetString("mysql_password")
	mysqlTables := conf.GetString("mysql_tables")

	if len(mysqlUser) == 0 {
		logger.Fatalf("the mysql_user property should not be empty")
	}
	if len(mysqlDB) == 0 {
		logger.Fatalf("the mysql_db property should not be empty")
	}
	if len(mysqlPassword) == 0 {
		logger.Fatalf("the mysql_password property should not be empty")
	}

	pool, cancelFunc, err := getMySQLPool(mysqlUser, mysqlDB, mysqlPassword)
	if err != nil {
		return err
	}
	defer cancelFunc()

	tablesToRead, err := getMySqlTables(mysqlTables, pool)
	if err != nil {
		return err
	}

	for _, table := range tablesToRead {
		if err = dumpTable(table, pool); err != nil {
			return err
		}
	}
	/*
		query := `select user_id, follower_user_id from user_relation`
		rows, err := pool.Query(query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var (
				userID         int64
				followerUserID int64
			)
			if err := rows.Scan(&userID, &followerUserID); err != nil {
				log.Fatal(err)
			}

			fmt.Printf("_:%s_%d <follows> _:%s_%d .\n", mysqlDB, followerUserID, mysqlDB, userID)
		}
	*/
	return nil
}
