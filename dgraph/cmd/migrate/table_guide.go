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
)

const (
	SEPERATOR = "_"
)

// A BlankNodeG generates the unique blank node label that corresponds to a Dgraph uid.
// Values are passed to the genBlankNode method in the order of alphabetically sorted columns
type BlankNodeG interface {
	genBlankNode(info *TableInfo, values []interface{}) string
}

// ColumnBNG generates blank node labels using values in the primary key columns
type ColumnBNG struct {
	primaryKeyIndices []*ColumnIdx
	separator         string
}

// As an example, if the employee table has 3 columns (f_name, l_name, and title),
// where f_name and l_name together form the primary key.
// Then a row with values John (f_name), Doe (l_name), Software Engineer (title)
// would generate a blank node label _:person_John_Doe using values from the primary key columns
// in the alphabetic order, that is f_name, l_name in this case.
func (g *ColumnBNG) genBlankNode(info *TableInfo, values []interface{}) string {
	if g.primaryKeyIndices == nil {
		g.primaryKeyIndices = getColumnIndices(info, func(info *TableInfo, column string) bool {
			return info.columns[column].keyType == PRIMARY
		})
	}

	// use the primary key indices to retrieve values in the current row
	valuesForKey := make([]string, 0)
	for _, columnIndex := range g.primaryKeyIndices {
		strVal, err := getValue(info.columns[columnIndex.name].dataType,
			values[columnIndex.index])
		if err != nil {
			logger.Fatalf("unable to get string value from primary key column %s", columnIndex.name)
		}
		valuesForKey = append(valuesForKey, strVal)
	}

	return fmt.Sprintf("_:%s%s%s", info.tableName, g.separator,
		strings.Join(valuesForKey, g.separator))
}

// A CounterBNG generates blank node labels using a row counter
type CounterBNG struct {
	rowCounter int
	separator  string
}

func (g *CounterBNG) genBlankNode(info *TableInfo, values []interface{}) string {
	g.rowCounter++
	return fmt.Sprintf("_:%s%s%d", info.tableName, g.separator, g.rowCounter)
}

// a ValuesRecorder remembers the mapping between an ref label and its blank node label
// For example, if the person table has the (fname, lname) as the primary key,
// and there are two unique indices on the columns "license" and "ssn" respectively.
// For the row fname (John), lname (Doe), license(101), ssn (999-999-9999)
// the Value recorder would remember the following mappings
// _:person_license_101 -> _:person_John_Doe
// _:person_ssn_999-999-9999 -> _:person_John_Doe
// It remembers these mapping so that if another table references the person table through foreign
// key constraints, there is a way to look up the blank node labels and use it to create
// a Dgraph link between the two rows in the two different tables.
type ValuesRecorder interface {
	record(info *TableInfo, values []interface{}, blankNodeLabel string)
	getBlankNode(indexLabel string) string
}

// for a given SQL row, the FKValuesRecorder records mappings from its foreign key target columns to
// the blank node of the row
type FKValuesRecorder struct {
	refToBlank map[string]string
	separator  string
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
func (r *FKValuesRecorder) record(info *TableInfo, values []interface{},
	blankNode string) {
	for _, cst := range info.cstSources {
		// for each foreign key cst, there should be a mapping
		cstColumns := getCstColumns(cst)
		cstColumnIndices := getColumnIndices(info,
			func(info *TableInfo, column string) bool {
				_, ok := cstColumns[column]
				return ok
			})

		refLabel, err := getRefLabel(&Ref{
			allColumns:       info.columns,
			refColumnIndices: cstColumnIndices,
			tableName:        info.tableName,
			separator:        r.separator,
			colValues:        values,
		})
		if err != nil {
			logger.Printf("ignoring the constraint because of error when getting ref label: %+v",
				cst)
			continue
		}
		r.refToBlank[refLabel] = blankNode
	}
}

func getCstColumns(cst *FKConstraint) map[string]interface{} {
	columnNames := make(map[string]interface{})
	for _, part := range cst.parts {
		columnNames[part.columnName] = struct{}{}
	}
	return columnNames
}

func getValue(dataType DataType, value interface{}) (string, error) {
	switch dataType {
	case STRING:
		return fmt.Sprintf("%s", value), nil
	case INT:
		//fmt.Printf("%+v %v -> NullInt64\n", value, value.(reflect.Value).Interface().(sql.NullInt64))
		intVal, err := value.(sql.NullInt64).Value()
		if err == nil {
			return fmt.Sprintf("%v", intVal), nil
		} else {
			return "", err
		}
		return fmt.Sprintf("%v", intVal), nil
	default:
		return fmt.Sprintf("%v", value), nil
	}
}

type Ref struct {
	allColumns       map[string]*ColumnInfo
	refColumnIndices []*ColumnIdx
	tableName        string
	separator        string
	colValues        []interface{}
}

func getRefLabel(ref *Ref) (string, error) {
	columnNameAndValues := make([]string, 0)
	for _, columnIdx := range ref.refColumnIndices {
		colVal, err := getValue(ref.allColumns[columnIdx.name].dataType,
			ref.colValues[columnIdx.index])
		if err != nil {
			return "", err
		}
		nameAndValue := fmt.Sprintf("%s%s%s", columnIdx.name,
			ref.separator, colVal)

		columnNameAndValues = append(columnNameAndValues,
			nameAndValue)
	}

	return fmt.Sprintf("_:%s%s%s", ref.tableName, ref.separator,
		strings.Join(columnNameAndValues, ref.separator)), nil
}

func (r *FKValuesRecorder) getBlankNode(indexLabel string) string {
	return r.refToBlank[indexLabel]
}

// an IdxG is responsible for generating Dgraph indices
type IdxG interface {
	genDgraphIndices(info *TableInfo) []string
}

// CompositeIdxG generates one Dgraph index per SQL table primary key
// or index. For a composite index in SQL, e.g. (fname, lname) in the person table, we will create
// separate indices for each individual column within Dgraph, e.g.
// person_fname: string @index(exact) .
// person_lname: string @index(exact) .
// The reason is that Dgraph does not support composite indices as of now.
type CompositeIdxG struct {
	separator string
}

func (g *CompositeIdxG) genDgraphIndices(info *TableInfo) []string {
	dgraphIndexes := make([]string, 0)
	sqlIndexedColumns := getColumnIndices(info, func(info *TableInfo, column string) bool {
		return info.columns[column].keyType != NONE
	})

	for _, column := range sqlIndexedColumns {
		predicate := fmt.Sprintf("%s%s%s", info.tableName, g.separator, column.name)

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
		pred := getPredFromConstraint(info.tableName, g.separator, cst)
		dgraphIndexes = append(dgraphIndexes, fmt.Sprintf("%s: [%s] .\n",
			pred, UID))
	}
	return dgraphIndexes
}

func getPredFromConstraint(
	tableName string, separator string, constraint *FKConstraint) string {
	columnNames := make([]string, 0)
	for _, part := range constraint.parts {
		columnNames = append(columnNames, part.columnName)
	}
	return fmt.Sprintf("%s%s%s_", tableName, separator, strings.Join(columnNames, separator))
}

// a PredNameG is responsible for generating pred names based on a table's info and a column name
type PredNameG interface {
	genPredicateName(info *TableInfo, column string) string
}

type SimplePredNameG struct {
	separator string
}

func (g *SimplePredNameG) genPredicateName(info *TableInfo, column string) string {
	return fmt.Sprintf("%s%s%s", info.tableName, g.separator, column)
}

type TableGuide struct {
	blankNodeG     BlankNodeG
	valuesRecorder ValuesRecorder
	indexG         IdxG
	predNameG      PredNameG
}

func getBlankNodeG(tableInfo *TableInfo) BlankNodeG {
	primaryKeyIndices := getColumnIndices(tableInfo, func(info *TableInfo, column string) bool {
		return info.columns[column].keyType == PRIMARY
	})

	if len(primaryKeyIndices) > 0 {
		return &ColumnBNG{
			separator: SEPERATOR,
		}
	}

	return &CounterBNG{
		separator: SEPERATOR,
	}
}

func getTableGuides(tables map[string]*TableInfo) map[string]*TableGuide {
	tableGuides := make(map[string]*TableGuide)
	for table, tableInfo := range tables {
		guide := &TableGuide{
			blankNodeG: getBlankNodeG(tableInfo),
			valuesRecorder: &FKValuesRecorder{
				refToBlank: make(map[string]string),
				separator:  SEPERATOR,
			},
			indexG: &CompositeIdxG{
				separator: SEPERATOR,
			},
			predNameG: &SimplePredNameG{
				separator: SEPERATOR,
			},
		}

		tableGuides[table] = guide
	}
	return tableGuides
}
