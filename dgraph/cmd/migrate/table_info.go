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

type KeyType int

const (
	NONE KeyType = iota
	PRIMARY
	MULTI
)

type DataType int

type ColumnInfo struct {
	name         string
	keyType      KeyType
	dataType     DataType
	referencedBy []*ForeignColumn //the columns that are referencing this column
}

type ForeignColumn struct {
	tableName  string
	columnName string
}

type TableInfo struct {
	tableName string
	columns   map[string]*ColumnInfo

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

	for prefix, goType := range mysqlTypePrefixToGoType {
		if strings.HasPrefix(columnOutput.dataType, prefix) {
			columnInfo.dataType = goType
			break
		}
	}
	return &columnInfo
}

func getTableInfo(table string, pool *sql.DB) (*TableInfo, error) {
	query := fmt.Sprintf(`select COLUMN_NAME,DATA_TYPE,
COLUMN_KEY from INFORMATION_SCHEMA.COLUMNS where TABLE_NAME = "%s"`, table)
	rows, err := pool.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tableInfo := &TableInfo{
		tableName:            table,
		columns:              make(map[string]*ColumnInfo),
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

		tableInfo.columns[columnOutput.fieldName] = getColumnInfo(&columnOutput)
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

// populateReferencedByColumns calculates the reverse links of
// the data at tables[table name].foreignKeyRefences
// and stores them in tables[table name].columns[column name].referecedBy
func populateReferencedByColumns(tables map[string]*TableInfo) {
	for table, tableInfo := range tables {
		for columnName, foreignColumn := range tableInfo.foreignKeyReferences {
			foreignTable := foreignColumn.tableName
			foreignColumn := foreignColumn.columnName

			foreignColumnInfo := tables[foreignTable].columns[foreignColumn]
			if foreignColumnInfo.referencedBy == nil {
				foreignColumnInfo.referencedBy = make([]*ForeignColumn, 0)
			}
			foreignColumnInfo.referencedBy = append(foreignColumnInfo.referencedBy, &ForeignColumn{
				tableName:  table,
				columnName: columnName,
			})
		}
	}
}
