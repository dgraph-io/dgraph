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

type ForeignColumn struct {
	tableName  string
	columnName string
}

type TableInfo struct {
	tableName string
	columns   []*ColumnInfo

	// the referenced tables by the current table through foreign keys
	referencedTables     map[string]interface{}
	foreignKeyReferences map[string]*ForeignColumn // a map from column names to ForgeignColumns
}

type ColumnOutput struct {
	fieldName string
	dataType  string
	keyType   string
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
	query := fmt.Sprintf(`select COLUMN_NAME,DATA_TYPE,
COLUMN_KEY from INFORMATION_SCHEMA.COLUMNS where TABLE_NAME = "%s"`, table)
	rows, err := pool.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tableInfo := &TableInfo{
		tableName:            table,
		columns:              make([]*ColumnInfo, 0),
		referencedTables:     make(map[string]interface{}),
		foreignKeyReferences: make(map[string]*ForeignColumn),
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
		err := rows.Scan(&columnOutput.fieldName, &columnOutput.dataType, &columnOutput.keyType)
		if err != nil {
			return nil, fmt.Errorf("unable to scan table description result for table %s: %v",
				table, err)
		}

		tableInfo.columns = append(tableInfo.columns, getColumnInfo(&columnOutput))
	}

	foreignKeysQuery := fmt.Sprintf(`select COLUMN_NAME, REFERENCED_TABLE_NAME,
		REFERENCED_COLUMN_NAME from INFORMATION_SCHEMA.KEY_COLUMN_USAGE where TABLE_NAME = "%s"
        AND REFERENCED_TABLE_NAME IS NOT NULL`,
		table)
	foreignKeyRows, err := pool.Query(foreignKeysQuery)
	if err != nil {
		return nil, err
	}
	defer foreignKeyRows.Close()
	for foreignKeyRows.Next() {
		/* example output from MySQL when querying the registration table
		+-------------+-----------------------+------------------------+
		| COLUMN_NAME | REFERENCED_TABLE_NAME | REFERENCED_COLUMN_NAME |
		+-------------+-----------------------+------------------------+
		| student_id  | student               | id                     |
		| course_id   | course                | id                     |
		| faculty_id  | faculty               | id                     |
		+-------------+-----------------------+------------------------+

		*/
		var columnName, referencedTableName, referencedColumnName string
		err := foreignKeyRows.Scan(&columnName, &referencedTableName, &referencedColumnName)
		if err != nil {
			return nil, fmt.Errorf("unable to scan usage info for table %s: %v", table, err)
		}

		tableInfo.referencedTables[referencedTableName] = struct{}{}
		tableInfo.foreignKeyReferences[columnName] = &ForeignColumn{
			tableName:  referencedTableName,
			columnName: referencedColumnName,
		}
	}
	return tableInfo, nil
}

func genGuide(conf *viper.Viper) error {
	mysqlUser := conf.GetString("mysql_user")
	mysqlDB := conf.GetString("mysql_db")
	mysqlPassword := conf.GetString("mysql_password")
	mysqlTables := conf.GetString("mysql_tables")
	outputFile := conf.GetString("output")

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
