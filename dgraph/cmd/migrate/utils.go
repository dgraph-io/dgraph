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
	"database/sql"
	"fmt"
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
