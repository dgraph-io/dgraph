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
	"sort"
	"strings"

	_ "github.com/go-sql-driver/mysql"
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
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("error while scanning table name: %v", err)
		}
		tables = append(tables, table)
	}

	return tables, nil
}

// getColumnIndices first sort the columns in the table alphabetically, and then
// returns the indices of the columns satisfying the criteria function
func getColumnIndices(info *TableInfo,
	criteria CriteriaFunc) []*ColumnIdx {
	columns := getSortedColumns(info)

	indices := make([]*ColumnIdx, 0)
	for i, column := range columns {
		if criteria(info, column) {
			indices = append(indices, &ColumnIdx{
				name:  column,
				index: i,
			})
		}
	}
	return indices
}

type ColumnIdx struct {
	name  string // the column name
	index int    // the column index
}

// ptrToValues takes a slice of pointers, deference them, and return the values referenced by these
// pointers
func ptrToValues(ptrs []interface{}) []interface{} {
	values := make([]interface{}, 0, len(ptrs))
	for _, ptr := range ptrs {
		// dereference the pointer to get the actual value
		v := reflect.ValueOf(ptr).Elem()
		values = append(values, v)
	}
	return values
}

func getFileWriter(filename string) (*bufio.Writer, func(), error) {
	output, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, nil, err
	}

	return bufio.NewWriter(output), func() { output.Close() }, nil
}

func getColumnValues(columns []string, columnTypes []*sql.ColumnType,
	rows *sql.Rows) ([]interface{}, error) {
	colValuePtrs := make([]interface{}, 0, len(columns))
	for i := 0; i < len(columns); i++ {
		switch columnTypes[i].DatabaseTypeName() {
		case "VARCHAR":
			colValuePtrs = append(colValuePtrs, new([]byte)) // the value can be nil
		case "INT":
			colValuePtrs = append(colValuePtrs, new(int))
		case "FLOAT":
			colValuePtrs = append(colValuePtrs, new(float64))
		default:
			panic(fmt.Sprintf("unknown type %v at index %d",
				columnTypes[i].ScanType().Kind(), i))
		}
	}
	if err := rows.Scan(colValuePtrs...); err != nil {
		return nil, fmt.Errorf("error while scanning column values: %v", err)
	}
	colValues := ptrToValues(colValuePtrs)
	return colValues, nil
}

// getSortedColumns sorts the column alphabetically using the column names
// and return the column names as a slice
func getSortedColumns(tableInfo *TableInfo) []string {
	columns := make([]string, 0)
	for column := range tableInfo.columns {
		columns = append(columns, column)
	}
	sort.Slice(columns, func(i, j int) bool {
		return columns[i] < columns[j]
	})
	return columns
}
