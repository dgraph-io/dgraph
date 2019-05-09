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

	"github.com/go-sql-driver/mysql"
)

var separator = "."

// A blankNode generates the unique blank node label that corresponds to a Dgraph uid.
// Values are passed to the genBlankNode method in the order of alphabetically sorted columns
type blankNode interface {
	generate(info *sqlTable, values []interface{}) string
}

// usingColumns generates blank node labels using values in the primary key columns
type usingColumns struct {
	primaryKeyIndices []*ColumnIdx
}

// As an example, if the employee table has 3 columns (f_name, l_name, and title),
// where f_name and l_name together form the primary key.
// Then a row with values John (f_name), Doe (l_name), Software Engineer (title)
// would generate a blank node label _:person_John_Doe using values from the primary key columns
// in the alphabetic order, that is f_name, l_name in this case.
func (g *usingColumns) generate(info *sqlTable, values []interface{}) string {
	if g.primaryKeyIndices == nil {
		g.primaryKeyIndices = getColumnIndices(info, func(info *sqlTable, column string) bool {
			return info.columns[column].keyType == primary
		})
	}

	// use the primary key indices to retrieve values in the current row
	var parts []string
	parts = append(parts, info.tableName)
	for _, columnIndex := range g.primaryKeyIndices {
		strVal, err := getValue(info.columns[columnIndex.name].dataType,
			values[columnIndex.index])
		if err != nil {
			logger.Fatalf("Unable to get string value from primary key column %s", columnIndex.name)
		}
		parts = append(parts, strVal)
	}

	return fmt.Sprintf("_:%s", strings.Join(parts, separator))
}

// A usingCounter generates blank node labels using a row counter
type usingCounter struct {
	rowCounter int
}

func (g *usingCounter) generate(info *sqlTable, values []interface{}) string {
	g.rowCounter++
	return fmt.Sprintf("_:%s%s%d", info.tableName, separator, g.rowCounter)
}

// a valuesRecorder remembers the mapping between an ref label and its blank node label
// For example, if the person table has the (fname, lname) as the primary key,
// and there are two unique indices on the columns "license" and "ssn" respectively.
// For the row fname (John), lname (Doe), license(101), ssn (999-999-9999)
// the Value recorder would remember the following mappings
// _:person_license_101 -> _:person_John_Doe
// _:person_ssn_999-999-9999 -> _:person_John_Doe
// It remembers these mapping so that if another table references the person table through foreign
// key constraints, there is a way to look up the blank node labels and use it to create
// a Dgraph link between the two rows in the two different tables.
type valuesRecorder interface {
	record(info *sqlTable, values []interface{}, blankNodeLabel string)
	getBlankNode(indexLabel string) string
}

// for a given SQL row, the fkValuesRecorder records mappings from its foreign key target columns to
// the blank node of the row
type fkValuesRecorder struct {
	refToBlank map[string]string
}

func (r *fkValuesRecorder) getBlankNode(indexLabel string) string {
	return r.refToBlank[indexLabel]
}

// record keeps track of the mapping between referenced foreign columns and the blank node label
// Consider the "person" table
// fname varchar(50)
// lname varchar(50)
// company varchar(50)
// employee_id int
// primary key (fname, lname)
// index unique (company, employee_id)

// and it is referenced by the "salary" table
// person_company varchar (50)
// person_employee_id int
// salary float
// foreign key (person_company, person_employee_id) references person (company, employee_id)

// Then the person table will have blank node label _:person_John_Doe for the row:
// John (fname), Doe (lname), Google (company), 100 (employee_id)
//
// And we need to record the mapping from the refLabel to the blank node label
// _:person_company_Google_employee_id_100 -> _:person_John_Doe
// This mapping will be used later, when processing the salary table, to find the blank node label
// _:person_John_Doe, which is used further to create the Dgraph link between a salary row
// and the person row
func (r *fkValuesRecorder) record(info *sqlTable, values []interface{},
	blankNode string) {
	for _, cst := range info.cstSources {
		// for each foreign key constraint, there should be a mapping
		cstColumns := getCstColumns(cst)
		cstColumnIndices := getColumnIndices(info,
			func(info *sqlTable, column string) bool {
				_, ok := cstColumns[column]
				return ok
			})

		refLabel, err := createLabel(&ref{
			allColumns:       info.columns,
			refColumnIndices: cstColumnIndices,
			tableName:        info.tableName,
			colValues:        values,
		})
		if err != nil {
			if !quiet {
				logger.Printf("ignoring the constraint because of error "+
					"when getting ref label: %+v\n", cst)
			}
			continue
		}
		r.refToBlank[refLabel] = blankNode
	}
}

func getCstColumns(cst *fkConstraint) map[string]interface{} {
	columnNames := make(map[string]interface{})
	for _, part := range cst.parts {
		columnNames[part.columnName] = struct{}{}
	}
	return columnNames
}

func getValue(dataType DataType, value interface{}) (string, error) {
	if value == nil {
		return "", fmt.Errorf("nil value found")
	}

	switch dataType {
	case STRING:
		return fmt.Sprintf("%s", value), nil
	case INT:
		if !value.(sql.NullInt64).Valid {
			return "", fmt.Errorf("found invalid nullint")
		}
		intVal, _ := value.(sql.NullInt64).Value()
		return fmt.Sprintf("%v", intVal), nil
	case DATETIME:
		if !value.(mysql.NullTime).Valid {
			return "", fmt.Errorf("found invalid nulltime")
		}
		dateVal, _ := value.(mysql.NullTime).Value()
		return fmt.Sprintf("%v", dateVal), nil
	case FLOAT:
		if !value.(sql.NullFloat64).Valid {
			return "", fmt.Errorf("found invalid nullfloat")
		}
		floatVal, _ := value.(sql.NullFloat64).Value()
		return fmt.Sprintf("%v", floatVal), nil
	default:
		return fmt.Sprintf("%v", value), nil
	}
}

type ref struct {
	allColumns       map[string]*columnInfo
	refColumnIndices []*ColumnIdx
	tableName        string
	colValues        []interface{}
}

func createLabel(ref *ref) (string, error) {
	parts := make([]string, 0)
	parts = append(parts, ref.tableName)
	for _, colIdx := range ref.refColumnIndices {
		colVal, err := getValue(ref.allColumns[colIdx.name].dataType,
			ref.colValues[colIdx.index])
		if err != nil {
			return "", err
		}
		parts = append(parts, colIdx.name, colVal)
	}

	return fmt.Sprintf("_:%s", strings.Join(parts, separator)), nil
}

// createDgraphSchema generates one Dgraph index per SQL primary key
// or index. For a composite index in SQL, e.g. (fname, lname) in the person table, we will create
// separate indices for each individual column within Dgraph, e.g.
// person_fname: string @index(exact) .
// person_lname: string @index(exact) .
// The reason is that Dgraph does not support composite indices as of now.
func createDgraphSchema(info *sqlTable) []string {
	dgraphIndexes := make([]string, 0)
	sqlIndexedColumns := getColumnIndices(info, func(info *sqlTable, column string) bool {
		return info.columns[column].keyType != none
	})

	for _, column := range sqlIndexedColumns {
		predicate := fmt.Sprintf("%s%s%s", info.tableName, separator, column.name)

		dataType := info.columns[column.name].dataType

		var index string
		if dataType == STRING {
			index = "@index(exact)"
		} else {
			index = fmt.Sprintf("@index(%s)", dataType)
		}

		dgraphIndexes = append(dgraphIndexes, fmt.Sprintf("%s: %s %s .\n",
			predicate, dataType, index))
	}

	for _, cst := range info.foreignKeyConstraints {
		pred := getPredFromConstraint(info.tableName, separator, cst)
		dgraphIndexes = append(dgraphIndexes, fmt.Sprintf("%s: [%s] .\n",
			pred, UID))
	}
	return dgraphIndexes
}

func getPredFromConstraint(
	tableName string, separator string, constraint *fkConstraint) string {
	columnNames := make([]string, 0)
	for _, part := range constraint.parts {
		columnNames = append(columnNames, part.columnName)
	}
	return fmt.Sprintf("%s%s%s%s", tableName, separator,
		strings.Join(columnNames, separator), separator)
}

func predicateName(info *sqlTable, column string) string {
	return fmt.Sprintf("%s%s%s", info.tableName, separator, column)
}

type tableGuide struct {
	blankNode      blankNode
	valuesRecorder valuesRecorder
}

func getBlankNodeGen(ti *sqlTable) blankNode {
	primaryKeyIndices := getColumnIndices(ti, func(info *sqlTable, column string) bool {
		return info.columns[column].keyType == primary
	})

	if len(primaryKeyIndices) > 0 {
		return &usingColumns{}
	}
	return &usingCounter{}
}

func getTableGuides(tables map[string]*sqlTable) map[string]*tableGuide {
	tableGuides := make(map[string]*tableGuide)
	for table, tableInfo := range tables {
		guide := &tableGuide{
			blankNode: getBlankNodeGen(tableInfo),
			valuesRecorder: &fkValuesRecorder{
				refToBlank: make(map[string]string),
			},
		}

		tableGuides[table] = guide
	}
	return tableGuides
}
