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
	"database/sql"
	"fmt"
	"strings"

	"github.com/dgraph-io/dgraph/x"
)

type KeyType int

const (
	NONE KeyType = iota
	PRIMARY
	MULTI
)

type DataType int

type ColumnInfo struct {
	name     string
	keyType  KeyType
	dataType DataType
}

// FKConstraint represents a foreign key constraint
type FKConstraint struct {
	parts []*ConstraintPart
	// the referenced column names and their indices in the foreign table
	foreignIndices []*ColumnIdx
}

type ConstraintPart struct {
	// the local table name
	tableName string
	// the local column name
	columnName string
	// the remote table name can be either the source or target of a foreign key constraint
	remoteTableName string
	// the remote column name can be either the source or target of a foreign key constraint
	remoteColumnName string
}

type TableInfo struct {
	tableName string
	columns   map[string]*ColumnInfo

	// the referenced tables by the current table through foreign key constraints
	referencedTables map[string]interface{}

	// a map from constraint names to constraints
	foreignKeyConstraints map[string]*FKConstraint

	// the list of foreign key constraints using this table as the target
	cstSources []*FKConstraint
}

type ColumnOutput struct {
	fieldName string
	dataType  string
}

func getColumnInfo(columnOutput *ColumnOutput) *ColumnInfo {
	columnInfo := ColumnInfo{}
	columnInfo.name = columnOutput.fieldName

	for prefix, goType := range mysqlTypePrefixToGoType {
		if strings.HasPrefix(columnOutput.dataType, prefix) {
			columnInfo.dataType = goType
			break
		}
	}
	return &columnInfo
}

func getTableInfo(table string, database string, pool *sql.DB) (*TableInfo, error) {
	query := fmt.Sprintf(`select COLUMN_NAME,DATA_TYPE from INFORMATION_SCHEMA.
COLUMNS where TABLE_NAME = "%s" AND TABLE_SCHEMA="%s"`, table,
		database)
	columnRows, err := pool.Query(query)
	if err != nil {
		return nil, err
	}
	defer columnRows.Close()

	tableInfo := &TableInfo{
		tableName:             table,
		columns:               make(map[string]*ColumnInfo),
		referencedTables:      make(map[string]interface{}),
		foreignKeyConstraints: make(map[string]*FKConstraint),
	}

	for columnRows.Next() {
		/*
			each row represents info about a column, for example
			+---------------+-----------+
			| COLUMN_NAME   | DATA_TYPE |
			+---------------+-----------+
			| p_company     | varchar   |
			| p_employee_id | int       |
			| p_fname       | varchar   |
			| p_lname       | varchar   |
			| title         | varchar   |
			+---------------+-----------+
		*/
		columnOutput := ColumnOutput{}
		err := columnRows.Scan(&columnOutput.fieldName, &columnOutput.dataType)
		if err != nil {
			return nil, fmt.Errorf("unable to scan table description result for table %s: %v",
				table, err)
		}

		tableInfo.columns[columnOutput.fieldName] = getColumnInfo(&columnOutput)
	}

	// query indices
	indexQuery := fmt.Sprintf(`select INDEX_NAME,COLUMN_NAME from INFORMATION_SCHEMA.`+
		`STATISTICS where TABLE_NAME = "%s" AND index_schema="%s"`, table, database)
	indexRows, err := pool.Query(indexQuery)
	if err != nil {
		return nil, err
	}
	defer indexRows.Close()
	for indexRows.Next() {
		var indexName, columnName string
		err := indexRows.Scan(&indexName, &columnName)
		if err != nil {
			return nil, fmt.Errorf("unable to scan index info for table %s: %v", table, err)
		}
		switch indexName {
		case "PRIMARY":
			tableInfo.columns[columnName].keyType = PRIMARY
		default:
			tableInfo.columns[columnName].keyType = MULTI
		}

	}

	foreignKeysQuery := fmt.Sprintf(`select COLUMN_NAME,CONSTRAINT_NAME,REFERENCED_TABLE_NAME,
		REFERENCED_COLUMN_NAME from INFORMATION_SCHEMA.KEY_COLUMN_USAGE where TABLE_NAME = "%s"
        AND CONSTRAINT_SCHEMA="%s" AND REFERENCED_TABLE_NAME IS NOT NULL`, table, database)
	foreignKeyRows, err := pool.Query(foreignKeysQuery)
	if err != nil {
		return nil, err
	}
	defer foreignKeyRows.Close()
	for foreignKeyRows.Next() {
		/* example output from MySQL
		+---------------+-----------------+-----------------------+------------------------+
		| COLUMN_NAME   | CONSTRAINT_NAME | REFERENCED_TABLE_NAME | REFERENCED_COLUMN_NAME |
		+---------------+-----------------+-----------------------+------------------------+
		| p_fname       | role_ibfk_1     | person                | fname                  |
		| p_lname       | role_ibfk_1     | person                | lname                  |
		| p_company     | role_ibfk_2     | person                | company                |
		| p_employee_id | role_ibfk_2     | person                | employee_id            |
		+---------------+-----------------+-----------------------+------------------------+
		*/
		var columnName, constraintName, referencedTableName, referencedColumnName string
		err := foreignKeyRows.Scan(&columnName, &constraintName, &referencedTableName,
			&referencedColumnName)
		if err != nil {
			return nil, fmt.Errorf("unable to scan usage info for table %s: %v", table, err)
		}

		tableInfo.referencedTables[referencedTableName] = struct{}{}
		var constraint *FKConstraint
		var ok bool
		if constraint, ok = tableInfo.foreignKeyConstraints[constraintName]; !ok {
			constraint = &FKConstraint{
				parts: make([]*ConstraintPart, 0),
			}
			tableInfo.foreignKeyConstraints[constraintName] = constraint
		}
		constraint.parts = append(constraint.parts, &ConstraintPart{
			tableName:        table,
			columnName:       columnName,
			remoteTableName:  referencedTableName,
			remoteColumnName: referencedColumnName,
		})
	}
	return tableInfo, nil
}

// validateAndGetReverse flip the foreign key reference direction in a constraint.
// For example, if the constraint's local table name is A, and it has 3 columns
// col1, col2, col3 that references a remote table B's 3 columns col4, col5, col6,
// then we return a reversed constraint whose local table name is B with local columns
// col4, col5, col6 whose remote table name is A, and remote columns are
// col1, col2 and col3
func validateAndGetReverse(constraint *FKConstraint) (string, *FKConstraint) {
	reverseParts := make([]*ConstraintPart, 0)
	// verify that within one constraint, the remote table names are the same
	var remoteTableName string
	for _, part := range constraint.parts {
		if len(remoteTableName) == 0 {
			remoteTableName = part.remoteTableName
		} else {
			x.AssertTrue(part.remoteTableName == remoteTableName)
		}
		reverseParts = append(reverseParts, &ConstraintPart{
			tableName:        part.remoteColumnName,
			columnName:       part.remoteColumnName,
			remoteTableName:  part.tableName,
			remoteColumnName: part.columnName,
		})
	}
	return remoteTableName, &FKConstraint{
		parts: reverseParts,
	}
}

// populateReferencedByColumns calculates the reverse links of
// the data at tables[table name].foreignKeyReferences
// and stores them in tables[table name].columns[column name].referencedBy
func populateReferencedByColumns(tables map[string]*TableInfo) {
	for _, tableInfo := range tables {
		for _, constraint := range tableInfo.foreignKeyConstraints {
			reverseTable, reverseConstraint := validateAndGetReverse(constraint)

			tables[reverseTable].cstSources = append(tables[reverseTable].
				cstSources, reverseConstraint)
		}
	}
}
