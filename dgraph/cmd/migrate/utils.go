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
	"os"
	"reflect"
	"strings"
)

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

// readMySqlTables will return a slice of table names using one of the following logic
// 1) if the parameter mysqlTables is not empty, this function will return a slice of table names
// by splitting the parameter with the separate comma
// 2) if the parameter is empty, this function will read all the tables under the given
// database from MySQL and then return the result

func readMySqlTables(mysqlTables string, pool *sql.DB) ([]string, error) {
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

// askUidColumn asks the user to type a column name, whole value will be used to generate
// uids in Dgraph, if the input is empty, then each SQL row will have a new uid in Dgraph
func askUidColumn(columnNameMap map[string]string) (string, error) {
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

// askPredicateForColumn asks the user to type a predicate name that will be used for the column
// if the input is empty, the columnName will be used as the predicate
func askPredicateForColumn(columnName string) (string, error) {
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

func getUidLabel(table string, column string, value interface{}) string {
	return fmt.Sprintf("_:%s_%s_%v", table, column, value)
}

func inferSubjectLabel(table string, columnNames []string, columnValues []interface{},
	tableGuide *TableGuide) string {
	if len(tableGuide.UidColumn) > 0 {
		for i := 0; i < len(columnNames); i++ {
			if columnNames[i] == tableGuide.UidColumn {
				return getUidLabel(table, columnNames[i], reflect.ValueOf(columnValues[i]).Elem())
			}
		}
		// we should never reach here
		logger.Fatalf("Unable to find the column name: %s", tableGuide.UidColumn)
	}

	tableGuide.TableRowNum++
	return getUidLabel(table, "", tableGuide.TableRowNum)
}
