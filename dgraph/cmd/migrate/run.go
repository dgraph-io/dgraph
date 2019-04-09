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
	"bufio"
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

type TableGuide struct {
	// optionally one column can be used as the uidColumn, whose values are mapped to Dgraph uids
	uidColumn string

	// mappedPredNames[i] stores the predicate name for value in column i of a MySQL table
	columnNameToPredicate map[string]string
}

// getUidColumn asks the user to type a column name, whole value will be used to generate
// uids in Dgraph, if the input is empty, then each SQL row will have a new uid in Dgraph
func getUidColumn(columnNameMap map[string]string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("choose a column to generate UIDs (hit ENTER for none):")
		columnInput, err := reader.ReadString('\n')
		columnInput = columnInput[:len(columnInput)-1]
		if err != nil {
			return "", err
		}
		if len(columnInput) == 0 {
			return "", nil
		}
		if _, ok := columnNameMap[columnInput]; ok {
			return columnInput, nil
		}
		fmt.Printf("the column %s does not exist, please try again\n", columnInput)
	}
}

// getPredicateForColumn asks the user to type a predicate name that will be used for the column
// if the input is empty, the columnName will be used as the predicate
func getPredicateForColumn(columnName string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("choose a predicate name for the column %s ("+
		"hit ENTER to use the exact column name):", columnName)
	predicate, err := reader.ReadString('\n')
	predicate = predicate[:len(predicate)-1]
	if len(predicate) == 0 {
		predicate = columnName
	}
	return predicate, err
}

func getTableGuide(table string, pool *sql.DB) (*TableGuide, error) {
	query := fmt.Sprintf(`describe %s`, table)
	rows, err := pool.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 6 columns will be returned for the describe command
	const PROPERTIES = 6
	// Field, Type, Null, Key, Default, Extra
	columnProperties := make([]interface{}, 0, PROPERTIES)
	for i := 0; i < PROPERTIES; i++ {
		columnProperties = append(columnProperties, new(string))
	}

	columnNameToPredicate := make(map[string]string)
	columnNames := make([]string, 0)
	for rows.Next() {
		rows.Scan(columnProperties...)
		columnName := reflect.ValueOf(columnProperties[0]).Elem().String()
		columnNameToPredicate[columnName] = ""
		columnNames = append(columnNames, columnName)
	}
	fmt.Printf("Columns in the table %s:\n", table)
	for _, colName := range columnNames {
		fmt.Printf("%s\n", colName)
	}

	tableGuide := &TableGuide{}
	tableGuide.uidColumn, err = getUidColumn(columnNameToPredicate)
	if err != nil {
		return nil, err
	}

	for _, colName := range columnNames {
		if colName == tableGuide.uidColumn {
			continue
		}
		columnNameToPredicate[colName], err = getPredicateForColumn(colName)
		if err != nil {
			return nil, err
		}
	}

	tableGuide.columnNameToPredicate = columnNameToPredicate
	return tableGuide, nil
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
		tableGuide, err := getTableGuide(table, pool)
		if err != nil {
			return err
		}
		fmt.Printf("%+v", tableGuide)
		/*
			if err = dumpTable(table, pool); err != nil {
				return err
			}*/
	}
	return nil
}
