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

const (
	separator = "_"
)

// A blankNodeGen generates the unique blank node label that corresponds to a Dgraph uid.
// Values are passed to the genBlankNode method in the order of alphabetically sorted columns
type blankNodeGen interface {
	genBlankNode(info *tableInfo, values []interface{}) string
}

// columnBNGen generates blank node labels using values in the primary key columns
type columnBNGen struct {
	primaryKeyIndices []*ColumnIdx
	separator         string
}

// As an example, if the employee table has 3 columns (f_name, l_name, and title),
// where f_name and l_name together form the primary key.
// Then a row with values John (f_name), Doe (l_name), Software Engineer (title)
// would generate a blank node label _:person_John_Doe using values from the primary key columns
// in the alphabetic order, that is f_name, l_name in this case.
func (g *columnBNGen) genBlankNode(info *tableInfo, values []interface{}) string {
	if g.primaryKeyIndices == nil {
		g.primaryKeyIndices = getColumnIndices(info, func(info *tableInfo, column string) bool {
			return info.columns[column].keyType == PRIMARY
		})
	}

	// use the primary key indices to retrieve values in the current row
	valuesForKey := make([]string, 0)
	for _, columnIndex := range g.primaryKeyIndices {
		strVal, err := getValue(info.columns[columnIndex.name].dataType,
			values[columnIndex.index])
		if err != nil {
			logger.Fatalf("Unable to get string value from primary key column %s", columnIndex.name)
		}
		valuesForKey = append(valuesForKey, strVal)
	}

	return fmt.Sprintf("_:%s%s%s", info.tableName, g.separator,
		strings.Join(valuesForKey, g.separator))
}

// A counterBNGen generates blank node labels using a row counter
type counterBNGen struct {
	rowCounter int
	separator  string
}

func (g *counterBNGen) genBlankNode(info *tableInfo, values []interface{}) string {
	g.rowCounter++
	return fmt.Sprintf("_:%s%s%d", info.tableName, g.separator, g.rowCounter)
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
	record(info *tableInfo, values []interface{}, blankNodeLabel string)
	getBlankNode(indexLabel string) string
}

// for a given SQL row, the fkValuesRecorder records mappings from its foreign key target columns to
// the blank node of the row
type fkValuesRecorder struct {
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
func (r *fkValuesRecorder) record(info *tableInfo, values []interface{},
	blankNode string) {
	for _, cst := range info.cstSources {
		// for each foreign key cst, there should be a mapping
		cstColumns := getCstColumns(cst)
		cstColumnIndices := getColumnIndices(info,
			func(info *tableInfo, column string) bool {
				_, ok := cstColumns[column]
				return ok
			})

		refLabel, err := getRefLabel(&ref{
			allColumns:       info.columns,
			refColumnIndices: cstColumnIndices,
			tableName:        info.tableName,
			separator:        r.separator,
			colValues:        values,
		})
		if err != nil {
			//logger.Printf("ignoring the constraint because of error when getting ref label: %+v",
			//cst)
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
	default:
		return fmt.Sprintf("%v", value), nil
	}
}

type ref struct {
	allColumns       map[string]*columnInfo
	refColumnIndices []*ColumnIdx
	tableName        string
	separator        string
	colValues        []interface{}
}

func getRefLabel(ref *ref) (string, error) {
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

func (r *fkValuesRecorder) getBlankNode(indexLabel string) string {
	return r.refToBlank[indexLabel]
}

// an IdxGen is responsible for generating Dgraph indices
type IdxGen interface {
	genDgraphIndices(info *tableInfo) []string
}

// compositeIdxGen generates one Dgraph index per SQL table primary key
// or index. For a composite index in SQL, e.g. (fname, lname) in the person table, we will create
// separate indices for each individual column within Dgraph, e.g.
// person_fname: string @index(exact) .
// person_lname: string @index(exact) .
// The reason is that Dgraph does not support composite indices as of now.
type compositeIdxGen struct {
	separator string
}

func (g *compositeIdxGen) genDgraphIndices(info *tableInfo) []string {
	dgraphIndexes := make([]string, 0)
	sqlIndexedColumns := getColumnIndices(info, func(info *tableInfo, column string) bool {
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
	tableName string, separator string, constraint *fkConstraint) string {
	columnNames := make([]string, 0)
	for _, part := range constraint.parts {
		columnNames = append(columnNames, part.columnName)
	}
	return fmt.Sprintf("%s%s%s_", tableName, separator, strings.Join(columnNames, separator))
}

// a predNameGen is responsible for generating pred names based on a table's info and a column name
type predNameGen interface {
	genPredicateName(info *tableInfo, column string) string
}

type simplePredNameGen struct {
	separator string
}

func (g *simplePredNameGen) genPredicateName(info *tableInfo, column string) string {
	return fmt.Sprintf("%s%s%s", info.tableName, g.separator, column)
}

type tableGuide struct {
	blankNodeGen   blankNodeGen
	valuesRecorder valuesRecorder
	indexGen       IdxGen
	predNameGen    predNameGen
}

func getBlankNodeGen(ti *tableInfo) blankNodeGen {
	primaryKeyIndices := getColumnIndices(ti, func(info *tableInfo, column string) bool {
		return info.columns[column].keyType == PRIMARY
	})

	if len(primaryKeyIndices) > 0 {
		return &columnBNGen{
			separator: separator,
		}
	}

	return &counterBNGen{
		separator: separator,
	}
}

func getTableGuides(tables map[string]*tableInfo) map[string]*tableGuide {
	tableGuides := make(map[string]*tableGuide)
	for table, tableInfo := range tables {
		guide := &tableGuide{
			blankNodeGen: getBlankNodeGen(tableInfo),
			valuesRecorder: &fkValuesRecorder{
				refToBlank: make(map[string]string),
				separator:  separator,
			},
			indexGen: &compositeIdxGen{
				separator: separator,
			},
			predNameGen: &simplePredNameGen{
				separator: separator,
			},
		}

		tableGuides[table] = guide
	}
	return tableGuides
}
