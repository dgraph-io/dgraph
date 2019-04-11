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

	"github.com/davecgh/go-spew/spew"

	"github.com/spf13/viper"
)

type KeyType int

const (
	NONE KeyType = iota
	PRIMARY
	MULTI
)

type DataType int

const (
	UNKNOWN DataType = iota
	INT
	STRING
	FLOAT
	DOUBLE
	DATETIME
)

type ColumnInfo struct {
	name     string
	keyType  KeyType
	dataType DataType
}

type TableInfo struct {
	tableName string
	columns   []*ColumnInfo
}

type ColumnOutput struct {
	fieldName string
	dataType  string
	nullable  string
	keyType   string
	defaults  string
	extra     string
}

func getColumnInfo(columnOutput *ColumnOutput) *ColumnInfo {
	columnInfo := ColumnInfo{}
	columnInfo.name = columnOutput.fieldName
	switch columnOutput.keyType {
	case "PRI":
		columnInfo.keyType = PRIMARY
	case "MUL":
		columnInfo.keyType = MULTI
	}

	prefixToType := make(map[string]DataType, 0)
	prefixToType["int"] = INT
	prefixToType["varchar"] = STRING
	prefixToType["date"] = DATETIME
	prefixToType["time"] = DATETIME
	prefixToType["datetime"] = DATETIME
	prefixToType["float"] = FLOAT
	prefixToType["double"] = DOUBLE

	for prefix, dataType := range prefixToType {
		if strings.HasPrefix(columnOutput.dataType, prefix) {
			columnInfo.dataType = dataType
			break
		}
	}
	return &columnInfo
}

func genTableInfo(table string, pool *sql.DB) (*TableInfo, error) {
	query := fmt.Sprintf(`describe %s`, table)
	rows, err := pool.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tableInfo := &TableInfo{
		tableName: table,
		columns:   make([]*ColumnInfo, 0),
	}

	for rows.Next() {
		/*
			each row represents info about a column, for example
			+----------+-------------+------+-----+---------+-------+
			| Field    | Type        | Null | Key | Default | Extra |
			+----------+-------------+------+-----+---------+-------+
			| ssn      | varchar(50) | NO   | PRI | NULL    |       |
		*/
		columnOutput := ColumnOutput{}
		rows.Scan(&columnOutput.fieldName, &columnOutput.dataType,
			&columnOutput.nullable, &columnOutput.keyType,
			&columnOutput.defaults, &columnOutput.extra)

		tableInfo.columns = append(tableInfo.columns, getColumnInfo(&columnOutput))
	}
	return tableInfo, nil
}

func genGuide(conf *viper.Viper) error {
	mysqlUser := conf.GetString("mysql_user")
	mysqlDB := conf.GetString("mysql_db")
	mysqlPassword := conf.GetString("mysql_password")
	mysqlTables := conf.GetString("mysql_tables")
	outputFile := conf.GetString("output")

	fmt.Printf("genGuide config file: %s", conf.GetString("config"))

	if len(mysqlUser) == 0 {
		logger.Fatalf("the mysql_user property should not be empty")
	}
	if len(mysqlDB) == 0 {
		logger.Fatalf("the mysql_db property should not be empty")
	}
	if len(mysqlPassword) == 0 {
		logger.Fatalf("the mysql_password property should not be empty")
	}
	if len(outputFile) == 0 {
		logger.Fatalf("the output file should not be empty")
	}

	/*
		output, err := os.OpenFile(outputFile, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		defer output.Close()
		ioWriter := bufio.NewWriter(output)
	*/

	pool, cancelFunc, err := getMySQLPool(mysqlUser, mysqlDB, mysqlPassword)
	if err != nil {
		return err
	}
	defer cancelFunc()

	tablesToRead, err := readMySqlTables(mysqlTables, pool)
	if err != nil {
		return err
	}

	tables := make(map[string]*TableInfo, 0)
	for _, table := range tablesToRead {
		tableInfo, err := genTableInfo(table, pool)
		if err != nil {
			return err
		}
		tables[tableInfo.tableName] = tableInfo
	}

	spew.Dump(tables)
	return nil
}
