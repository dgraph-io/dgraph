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

type keyType int

const (
	none keyType = iota
	primary
	secondary
)

type DataType int

type columnInfo struct {
	name     string
	keyType  keyType
	dataType DataType
}

// fkConstraint represents a foreign key constraint
type fkConstraint struct {
	parts []*constraintPart
	// the referenced column names and their indices in the foreign table
	foreignIndices []*ColumnIdx
}

type constraintPart struct {
	// the local table name
	tableName string
	// the local column name
	columnName string
	// the remote table name can be either the source or target of a foreign key constraint
	remoteTableName string
	// the remote column name can be either the source or target of a foreign key constraint
	remoteColumnName string
}

// a sqlTable contains a SQL table's metadata such as the table name,
// the info of each column etc
type sqlTable struct {
	tableName string
	columns   map[string]*columnInfo

	// The following 3 columns are used by the rowMeta when converting rows
	columnDataTypes []DataType
	columnNames     []string
	predNames       []string

	// the referenced tables by the current table through foreign key constraints
	dstTables map[string]interface{}

	// a map from constraint names to constraints
	foreignKeyConstraints map[string]*fkConstraint

	// the list of foreign key constraints using this table as the target
	cstSources []*fkConstraint
}

func getDataType(dbType string) DataType {
	for prefix, goType := range sqlTypeToInternal {
		if strings.HasPrefix(dbType, prefix) {
			return goType
		}
	}
	return UNKNOWN
}

func getColumnInfo(fieldName string, dbType string) *columnInfo {
	columnInfo := columnInfo{}
	columnInfo.name = fieldName
	columnInfo.dataType = getDataType(dbType)
	return &columnInfo
}

func parseTables(pool *sql.DB, tableName string, database string) (*sqlTable, error) {
	query := fmt.Sprintf(`select COLUMN_NAME,DATA_TYPE from INFORMATION_SCHEMA.
COLUMNS where TABLE_NAME = "%s" AND TABLE_SCHEMA="%s" ORDER BY COLUMN_NAME`, tableName, database)
	columns, err := pool.Query(query)
	if err != nil {
		return nil, err
	}
	defer columns.Close()

	table := &sqlTable{
		tableName:             tableName,
		columns:               make(map[string]*columnInfo),
		columnNames:           make([]string, 0),
		columnDataTypes:       make([]DataType, 0),
		predNames:             make([]string, 0),
		dstTables:             make(map[string]interface{}),
		foreignKeyConstraints: make(map[string]*fkConstraint),
	}

	for columns.Next() {
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
		var fieldName, dbType string
		if err := columns.Scan(&fieldName, &dbType); err != nil {
			return nil, fmt.Errorf("unable to scan table description result for table %s: %v",
				tableName, err)
		}

		// TODO, should store the column data types into the table info as an array
		// and the RMI should simply get the data types from the table info
		table.columns[fieldName] = getColumnInfo(fieldName, dbType)
		table.columnNames = append(table.columnNames, fieldName)
		table.columnDataTypes = append(table.columnDataTypes, getDataType(dbType))
	}

	// query indices
	indexQuery := fmt.Sprintf(`select INDEX_NAME,COLUMN_NAME from INFORMATION_SCHEMA.`+
		`STATISTICS where TABLE_NAME = "%s" AND index_schema="%s"`, tableName, database)
	indices, err := pool.Query(indexQuery)
	if err != nil {
		return nil, err
	}
	defer indices.Close()
	for indices.Next() {
		var indexName, columnName string
		err := indices.Scan(&indexName, &columnName)
		if err != nil {
			return nil, fmt.Errorf("unable to scan index info for table %s: %v", tableName, err)
		}
		switch indexName {
		case "PRIMARY":
			table.columns[columnName].keyType = primary
		default:
			table.columns[columnName].keyType = secondary
		}

	}

	foreignKeysQuery := fmt.Sprintf(`select COLUMN_NAME,CONSTRAINT_NAME,REFERENCED_TABLE_NAME,
		REFERENCED_COLUMN_NAME from INFORMATION_SCHEMA.KEY_COLUMN_USAGE where TABLE_NAME = "%s"
        AND CONSTRAINT_SCHEMA="%s" AND REFERENCED_TABLE_NAME IS NOT NULL`, tableName, database)
	fkeys, err := pool.Query(foreignKeysQuery)
	if err != nil {
		return nil, err
	}
	defer fkeys.Close()
	for fkeys.Next() {
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
		var col, constraintName, dstTable, dstCol string
		if err := fkeys.Scan(&col, &constraintName, &dstTable, &dstCol); err != nil {
			return nil, fmt.Errorf("unable to scan usage info for table %s: %v", tableName, err)
		}

		table.dstTables[dstTable] = struct{}{}
		var constraint *fkConstraint
		var ok bool
		if constraint, ok = table.foreignKeyConstraints[constraintName]; !ok {
			constraint = &fkConstraint{
				parts: make([]*constraintPart, 0),
			}
			table.foreignKeyConstraints[constraintName] = constraint
		}
		constraint.parts = append(constraint.parts, &constraintPart{
			tableName:        tableName,
			columnName:       col,
			remoteTableName:  dstTable,
			remoteColumnName: dstCol,
		})
	}
	return table, nil
}

// validateAndGetReverse flip the foreign key reference direction in a constraint.
// For example, if the constraint's local table name is A, and it has 3 columns
// col1, col2, col3 that references a remote table B's 3 columns col4, col5, col6,
// then we return a reversed constraint whose local table name is B with local columns
// col4, col5, col6 whose remote table name is A, and remote columns are
// col1, col2 and col3
func validateAndGetReverse(constraint *fkConstraint) (string, *fkConstraint) {
	reverseParts := make([]*constraintPart, 0)
	// verify that within one constraint, the remote table names are the same
	var remoteTableName string
	for _, part := range constraint.parts {
		if len(remoteTableName) == 0 {
			remoteTableName = part.remoteTableName
		} else {
			x.AssertTrue(part.remoteTableName == remoteTableName)
		}
		reverseParts = append(reverseParts, &constraintPart{
			tableName:        part.remoteColumnName,
			columnName:       part.remoteColumnName,
			remoteTableName:  part.tableName,
			remoteColumnName: part.columnName,
		})
	}
	return remoteTableName, &fkConstraint{
		parts: reverseParts,
	}
}

// populateReferencedByColumns calculates the reverse links of
// the data at tables[table name].foreignKeyReferences
// and stores them in tables[table name].cstSources
func populateReferencedByColumns(tables map[string]*sqlTable) {
	for _, tableInfo := range tables {
		for _, constraint := range tableInfo.foreignKeyConstraints {
			reverseTable, reverseConstraint := validateAndGetReverse(constraint)

			tables[reverseTable].cstSources = append(tables[reverseTable].cstSources,
				reverseConstraint)
		}
	}
}
