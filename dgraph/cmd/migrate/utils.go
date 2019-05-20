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
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
)

func getPool(user string, db string, password string) (*sql.DB,
	error) {
	return sql.Open("mysql",
		fmt.Sprintf("%s:%s@/%s?parseTime=true", user, password, db))
}

// showTables will return a slice of table names using one of the following logic
// 1) if the parameter tables is not empty, this function will return a slice of table names
// by splitting the parameter with the separate comma
// 2) if the parameter is empty, this function will read all the tables under the given
// database and then return the result
func showTables(pool *sql.DB, tableNames string) ([]string, error) {
	if len(tableNames) > 0 {
		return strings.Split(tableNames, ","), nil
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

type criteriaFunc func(info *sqlTable, column string) bool

// getColumnIndices first sort the columns in the table alphabetically, and then
// returns the indices of the columns satisfying the criteria function
func getColumnIndices(info *sqlTable, criteria criteriaFunc) []*ColumnIdx {
	indices := make([]*ColumnIdx, 0)
	for i, column := range info.columnNames {
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

func getFileWriter(filename string) (*bufio.Writer, func(), error) {
	output, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, nil, err
	}

	return bufio.NewWriter(output), func() { output.Close() }, nil
}

func getColumnValues(columns []string, dataTypes []DataType,
	rows *sql.Rows) ([]interface{}, error) {
	// ptrToValues takes a slice of pointers, deference them, and return the values referenced
	// by these pointers
	ptrToValues := func(ptrs []interface{}) []interface{} {
		values := make([]interface{}, 0, len(ptrs))
		for _, ptr := range ptrs {
			// dereference the pointer to get the actual value
			v := reflect.ValueOf(ptr).Elem().Interface()
			values = append(values, v)
		}
		return values
	}

	valuePtrs := make([]interface{}, 0, len(columns))
	for i := 0; i < len(columns); i++ {
		switch dataTypes[i] {
		case STRING:
			valuePtrs = append(valuePtrs, new([]byte)) // the value can be nil
		case INT:
			valuePtrs = append(valuePtrs, new(sql.NullInt64))
		case FLOAT:
			valuePtrs = append(valuePtrs, new(sql.NullFloat64))
		case DATETIME:
			valuePtrs = append(valuePtrs, new(mysql.NullTime))
		default:
			panic(fmt.Sprintf("detected unsupported type %s on column %s",
				dataTypes[i], columns[i]))
		}
	}
	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, fmt.Errorf("error while scanning column values: %v", err)
	}
	colValues := ptrToValues(valuePtrs)
	return colValues, nil
}
