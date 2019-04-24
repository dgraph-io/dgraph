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
	"bufio"
	"database/sql"
	"fmt"
	"strings"
)

// DumpMeta serves as the global knowledge oracle that stores
// all the tables' info,
// all the tables' generation guide,
// the writer to output the generated RDF entries,
// the writer to output the Dgraph schema,
// and a sqlPool to read information from MySQL
type DumpMeta struct {
	tableInfos   map[string]*TableInfo
	tableGuides  map[string]*TableGuide
	dataWriter   *bufio.Writer
	schemaWriter *bufio.Writer
	sqlPool      *sql.DB
}

// RowMetaInfo captures values in a SQL table row, as well as the metadata associated
// with the row
type RowMetaInfo struct {
	colValues      []interface{}
	predNames      []string
	columnNames    []string
	blankNodeLabel string
	columnTypes    []*sql.ColumnType
	tableInfo      *TableInfo
}

// dumpSchema generates the Dgraph schema based on m.tableGuides
// and sends the schema to m.schemaWriter
func (m *DumpMeta) dumpSchema() error {
	for table, guide := range m.tableGuides {
		tableInfo := m.tableInfos[table]
		for _, index := range guide.indexG.genDgraphIndices(tableInfo) {
			_, err := m.schemaWriter.WriteString(index)
			if err != nil {
				return fmt.Errorf("error while writing schema: %v", err)
			}
		}
	}
	return m.schemaWriter.Flush()
}

// dumpTables runs a topological sort through all the tables in m.tableInfos,
// then this method dumps all the tables following that topological order where
// the most deeply referenced tables are processed first, and the non-referenced tables
// are processed later
func (m *DumpMeta) dumpTables() error {
	tablesSorted, err := topoSortTables(m.tableInfos)
	if err != nil {
		return err
	}

	for _, table := range tablesSorted {
		fmt.Printf("Dumping table %s\n", table)
		if err := m.dumpTable(table); err != nil {
			return fmt.Errorf("error while dumping table %s: %v", table, err)
		}
	}

	return m.dataWriter.Flush()
}

// dumpTable reads data from a table and sends generated RDF entries to the m.dataWriter
func (m *DumpMeta) dumpTable(table string) error {
	tableGuide := m.tableGuides[table]
	tableInfo := m.tableInfos[table]
	sortedColumns := getSortedColumns(tableInfo)

	query := fmt.Sprintf(`select %s from %s`, strings.Join(sortedColumns, ","), table)
	rows, err := m.sqlPool.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var rmi *RowMetaInfo
	for rows.Next() {
		if rmi == nil {
			rmi, err = getRowMetaInfo(rows, tableGuide, tableInfo)
			if err != nil {
				return fmt.Errorf("unable to get column types and pred names: %v", err)
			}
		}

		// step 1: read the row's column values
		colValues, err := getColumnValues(rmi.columnNames, rmi.columnTypes, rows)
		if err != nil {
			return err
		}
		rmi.colValues = colValues

		// step 2: output the column values in RDF format
		rmi.blankNodeLabel = tableGuide.blankNodeG.genBlankNode(tableInfo, colValues)

		m.outputRow(rmi, tableInfo)

		// step 3: record mappings to the blankNodeLabel so that future tables can look up the
		// blankNodeLabel
		tableGuide.valuesRecorder.record(tableInfo, colValues, rmi.blankNodeLabel)
	}
	return nil
}

// getRowMetaInfo returns a SQL row's column names, column types, and
// the predicate names that the column values should be stored at.
// For example, given the table person with the following schema
// fname varchar(50)
// lname varchar(50)
// this function will return the following tuple
// ([fname, lname], [VARCHAR, VARCHAR], [person_fname, person_lname])
func getRowMetaInfo(rows *sql.Rows, tableGuide *TableGuide,
	tableInfo *TableInfo) (*RowMetaInfo, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	// initialize the predNames
	var columnPredNames []string
	for _, column := range columns {
		columnPredNames = append(columnPredNames,
			tableGuide.predNameG.genPredicateName(tableInfo, column))
	}
	return &RowMetaInfo{
		columnNames: columns,
		columnTypes: columnTypes,
		predNames:   columnPredNames,
		tableInfo:   tableInfo,
	}, nil
	//columns, columnTypes, predNames, nil
}

// outputRow takes a row with its metadata as well as the table metadata, and
// spits out one or more RDF entries to the DumpMeta's dataWriter.
// Consider the following table "salary"
// person_company varchar (50)
// person_employee_id int
// salary float
// foreign key (person_company, person_employee_id) references person (company, employee_id)

// A row with the following values in the table
// Google, 100, 50.0 (salary)
// where Google is the person_company, 100 is the employee id, and 50.0 is the salary rate
// will cause the following RDF entries to be generated
// _:salary_1 <salary_person_company> "Google" .
// _:salary_1 <salary_person_employee_id> "100" .
// _:salary_1 <salary_person_salary> "50.0" .
// _:salary_1 <salary_person_company_person_employee_id> _:person_2.
// In the RDF output, _:salary_1 is this row's blank node label;
// salary_person_company, salary_person_employee_id, and salary_person_salary
// are the predicate names constructed by appending the column names after the table name "salary.

// The last RDF entry is a Dgraph edge created by following the foreign key reference.
// Its predicate name is constructed by concatenating the table name, and each column's name in
// alphabetical order. The object _:person_2 is the blank node label from the person table,
// and it's generated by a lookup using the key, called "ref label",
// _:person_company_Google_employee_id_100. The mapping from the ref label
// _:person_company_Google_employee_id_100 to the foreign blank node _:person_2
// is recorded through the valuesRecorder when the person table is processed.
func (m *DumpMeta) outputRow(rmi *RowMetaInfo, tableInfo *TableInfo) {
	for i, colValue := range rmi.colValues {
		predicate := rmi.predNames[i]
		outputPlainCell(rmi.blankNodeLabel, rmi.columnTypes[i].DatabaseTypeName(),
			predicate,
			colValue, m.dataWriter)
	}

	for _, constraint := range tableInfo.foreignKeyConstraints {
		if len(constraint.parts) == 0 {
			logger.Fatalf("The constraint should have at least one part: %v", constraint)
		}

		foreignTableName := constraint.parts[0].remoteTableName

		refLabel := rmi.getRefLabelFromConstraint(m.tableInfos[foreignTableName], constraint)
		foreignBlankNode := m.tableGuides[foreignTableName].valuesRecorder.getBlankNode(refLabel)
		outputPlainCell(rmi.blankNodeLabel, "UID",
			getPredFromConstraint(tableInfo.tableName, SEPERATOR, constraint),
			foreignBlankNode, m.dataWriter)
	}
}

// outputPlainCell sends to the writer a RDF where the subject is the uidLabel
// the predicate is the predName, and the object is the colValue
func outputPlainCell(uidLabel string, dbType string, predName string,
	colValue interface{}, writer *bufio.Writer) {
	// each cell value should be stored under a predicate
	fmt.Fprintf(writer, "%s <%s> ", uidLabel, predName)
	switch dbType {
	case "VARCHAR":
		fmt.Fprintf(writer, "%q .\n", colValue)
	case "UID":
		fmt.Fprintf(writer, "%s .\n", colValue)
	default:
		fmt.Fprintf(writer, "\"%v\" .\n", colValue)
	}
}

// getRefLabelFromConstraint returns a ref label based on a foreign key constraint.
// Consider the foreign key constraint
// foreign key (person_company, person_employee_id) references person (company, employee_id)

// and a row with the following values in the table
// Google, 100, 50.0 (salary)
// where Google is the person_company, 100 is the employee id, and 50.0 is the salary rate
// the refLabel will use the foreign table name, foreign column names and the local row's values,
// having the value of _:person_company_Google_employee_id_100
func (rmi *RowMetaInfo) getRefLabelFromConstraint(foreignTableInfo *TableInfo,
	constraint *FKConstraint) string {
	if constraint.foreignIndices == nil {
		foreignKeyColumnNames := make(map[string]string)
		for _, part := range constraint.parts {
			foreignKeyColumnNames[part.columnName] = part.remoteColumnName
		}

		constraint.foreignIndices = getColumnIndices(rmi.tableInfo,
			func(info *TableInfo, column string) bool {
				_, ok := foreignKeyColumnNames[column]
				return ok
			})

		// replace the column names to be the foreign column names
		for _, columnIdx := range constraint.foreignIndices {
			columnIdx.name = foreignKeyColumnNames[columnIdx.name]
		}
	}

	return getAliasLabel(&Alias{
		allColumns:         foreignTableInfo.columns,
		aliasColumnIndices: constraint.foreignIndices,
		tableName:          foreignTableInfo.tableName,
		separator:          SEPERATOR,
		colValues:          rmi.colValues,
	})
}
