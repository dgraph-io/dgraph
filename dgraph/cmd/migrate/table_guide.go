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
	"fmt"
	"strings"
)

const (
	SEPERATOR = "_"
)

// A BlankNodeGenerator generates the unique blank node label that corresponds to a Dgraph uid.
// Values are passed to the generateBlankNode method in the order of alphabetically sorted columns
type BlankNodeGenerator interface {
	generateBlankNode(info *TableInfo, values []interface{}) string
}

// generate blank node labels using values in the primary key columns
type ColumnKeyGenerator struct {
	primaryKeyIndices []*ColumnIdx
	separator         string
}

type CriteriaFunc func(info *TableInfo, column string) bool

// For example, if the employee table has 3 columns (f_name, l_name, and title),
// where f_name and l_name together form the primary key.
// Then a row with values John (f_name), Doe (l_name), Software Engineer (title)
// would generate a blank node label _:person_John_Doe using values from the columns
// of the primary key in the alphabetic order, i.e. f_name, l_name in this case.
func (g *ColumnKeyGenerator) generateBlankNode(info *TableInfo, values []interface{}) string {
	if g.primaryKeyIndices == nil {
		g.primaryKeyIndices = getColumnIndices(info, func(info *TableInfo, column string) bool {
			return info.columns[column].keyType == PRIMARY
		})
	}

	// use the primary key indices to retrieve values in the current row
	valuesForKey := make([]string, 0)
	for _, columnIndex := range g.primaryKeyIndices {
		valuesForKey = append(valuesForKey,
			getValue(info.columns[columnIndex.name].dataType,
				values[columnIndex.index]))
		//			fmt.Sprintf("%v", values[columnIndex.index])
	}

	return fmt.Sprintf("_:%s%s%s", info.tableName, g.separator,
		strings.Join(valuesForKey, g.separator))
}

// generate blank node labels using a row counter
type CounterKeyGenerator struct {
	rowCounter int
	separator  string
}

func (g *CounterKeyGenerator) generateBlankNode(info *TableInfo, values []interface{}) string {
	g.rowCounter++
	return fmt.Sprintf("_:%s%s%d", info.tableName, g.separator, g.rowCounter)
}

// a ValuesRecorder remembers the mapping between an alias and its blank node label
// For example, if the person table has the (fname, lname) as the primary key,
// and hence the blank node labels are like _:person_<first name value>_<last name value>,
// there are two unique indices on the columns license, and (ssn) respectively.
// For the row fname (John), lname (Doe), license(101), ssn (999-999-9999)
// the Value recorder would remember the following mappings
// _:person_license_101 -> _:person_John_Doe
// _:person_ssn_999-999-9999 -> _:person_John_Doe
// It remembers these mapping so that if another table references the person table through foreign
// keys, it will be able to look up the blank node labels and use it to establish links in Dgraph
type ValuesRecorder interface {
	record(info *TableInfo, values []interface{}, uidLabel string)
	getUidLabel(indexLabel string) string
}

type ForeignKeyValuesRecorder struct {
	referenceToUidLabel map[string]string
	separator           string
}

/*
TODO: for now we do NOT support composite foreign keys
For example, if we have the following two tables where <fname, lname> combined serves as the
foreign key,

create table person (
fname varchar(50),
lname varchar(50),
INDEX (fname, lname)
);

create table role (
title varchar(50),
p_fname varchar(50),
p_lname varchar(50),
FOREIGN KEY (p_fname, p_lname) REFERENCES person (fname, lname)
);

the tool will treat them as two different foreign keys, where the p_fname references person fname,
and p_lname references person lname.
*/
func (r *ForeignKeyValuesRecorder) record(info *TableInfo, values []interface{}, uidLabel string) {
	/*
		if r.referencedByColumnIndices == nil {
			r.referencedByColumnIndices = getColumnIndices(info, func(info *TableInfo, column string) bool {
				return info.columns[column].isForeignKeyTarget
			})
		}

		for _, columnIndex := range r.referencedByColumnIndices {
			referenceLabel := fmt.Sprintf("_:%s%s%s%s%v", info.tableName,
				r.separator, columnIndex.name, r.separator, values[columnIndex.index])
			r.referenceToUidLabel[referenceLabel] = uidLabel
		}
	*/

	for _, constraint := range info.constraintSources {
		// for each foreign key constraint, there should be a mapping
		constraintColumns := getConstraintColumns(constraint)
		constraintColumnIndices := getColumnIndices(info, func(info *TableInfo, column string) bool {
			_, ok := constraintColumns[column]
			return ok
		})

		aliasLabel := getAliasLabel(info.columns, info.tableName, r.separator, constraintColumnIndices, values)
		r.referenceToUidLabel[aliasLabel] = uidLabel
		fmt.Printf("%s -> %s\n", aliasLabel, uidLabel)
	}
}

func getConstraintColumns(constraint *ForeignKeyConstraint) map[string]interface{} {
	columnNames := make(map[string]interface{})
	for _, part := range constraint.parts {
		columnNames[part.columnName] = struct{}{}
	}
	return columnNames
}

func getValue(dataType DataType, value interface{}) string {
	switch dataType {
	case STRING:
		return fmt.Sprintf("%s", value)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func getAliasLabel(columnMaps map[string]*ColumnInfo, tableName string, separator string,
	columnIndices []*ColumnIdx,
	values []interface{}) string {

	columnNameAndValues := make([]string, 0)
	for _, columnIdx := range columnIndices {
		nameAndValue := fmt.Sprintf("%s%s%s", columnIdx.name,
			separator,
			getValue(columnMaps[columnIdx.name].dataType, values[columnIdx.index]))

		columnNameAndValues = append(columnNameAndValues,
			nameAndValue)
	}

	return fmt.Sprintf("_:%s%s%s", tableName, separator, strings.Join(columnNameAndValues,
		separator))
}

func (r *ForeignKeyValuesRecorder) getUidLabel(indexLabel string) string {
	fmt.Printf("looking up %s\n", indexLabel)
	return r.referenceToUidLabel[indexLabel]
}

type IndexGenerator interface {
	generateDgraphIndices(info *TableInfo) []string
}

// CompositeIndexGenerator generates one Dgraph index per SQL table primary key
// or index, where only the first column in the primary key or index will be used
type CompositeIndexGenerator struct {
	separator string
}

func (g *CompositeIndexGenerator) generateDgraphIndices(info *TableInfo) []string {
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

	for _, constraint := range info.foreignKeyConstraints {
		pred := getPredFromConstraint(info.tableName, g.separator, constraint)
		dgraphIndexes = append(dgraphIndexes, fmt.Sprintf("%s: [%s] .\n",
			pred, UID))
	}
	return dgraphIndexes
}

func getPredFromConstraint(
	tableName string, separator string, constraint *ForeignKeyConstraint) string {
	columnNames := make([]string, 0)
	for _, part := range constraint.parts {
		columnNames = append(columnNames, part.columnName)
	}
	return fmt.Sprintf("%s%s%s", tableName, separator, strings.Join(columnNames, separator))
}

type PredNameGenerator interface {
	generatePredicateName(info *TableInfo, column string) string
}

type SimplePredNameGenerator struct {
	separator string
}

func (g *SimplePredNameGenerator) generatePredicateName(info *TableInfo, column string) string {
	return fmt.Sprintf("%s%s%s", info.tableName, g.separator, column)
}

type TableGuide struct {
	blankNodeGenerator BlankNodeGenerator
	valuesRecorder     ValuesRecorder
	indexGenerator     IndexGenerator
	predNameGenerator  PredNameGenerator
}

func getKeyGenerator(tableInfo *TableInfo) BlankNodeGenerator {
	// check if the table has primary keys
	primaryKeyIndices := getColumnIndices(tableInfo, func(info *TableInfo, column string) bool {
		return info.columns[column].keyType == PRIMARY
	})

	if len(primaryKeyIndices) > 0 {
		return &ColumnKeyGenerator{
			separator: SEPERATOR,
		}
	}

	return &CounterKeyGenerator{
		separator: SEPERATOR,
	}
}

func getTableGuides(tables map[string]*TableInfo) map[string]*TableGuide {
	tableGuides := make(map[string]*TableGuide)
	for table, tableInfo := range tables {
		guide := &TableGuide{
			blankNodeGenerator: getKeyGenerator(tableInfo),
			valuesRecorder: &ForeignKeyValuesRecorder{
				referenceToUidLabel: make(map[string]string),
				separator:           SEPERATOR,
			},
			indexGenerator: &CompositeIndexGenerator{
				separator: SEPERATOR,
			},
			predNameGenerator: &SimplePredNameGenerator{
				separator: SEPERATOR,
			},
		}

		tableGuides[table] = guide
	}
	return tableGuides
}
