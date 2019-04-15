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
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"strings"
)

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
	colValues       []interface{}
	columnPredNames []string
	columnNames     []string
	blankNodeLabel  string
	columnTypes     []*sql.ColumnType
}

func (m *DumpMeta) dumpSchema() error {
	for table, guide := range m.tableGuides {
		tableInfo := m.tableInfos[table]
		for _, index := range guide.indexGenerator.generateDgraphIndices(tableInfo) {
			_, err := m.schemaWriter.WriteString(index)
			if err != nil {
				return fmt.Errorf("error while writing schema: %v", err)
			}
		}
	}
	return m.schemaWriter.Flush()
}

func (m *DumpMeta) dumpTables() error {
	tablesSorted, err := topoSortTables(m.tableInfos)
	if err != nil {
		return err
	}

	for _, table := range tablesSorted {
		fmt.Printf("Dumping table %s\n", table)
		m.dumpTable(table)
	}

	return m.dataWriter.Flush()
}

// dumpTable reads data from a table and sends to the writer
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

	var columnNames []string
	var columnTypes []*sql.ColumnType
	var columnPredNames []string
	for rows.Next() {
		if columnNames == nil {
			columnNames, columnTypes, columnPredNames, err = getColumnTypesAndPreNames(rows,
				tableGuide, tableInfo)
			if err != nil {
				return fmt.Errorf("unable to get column types and pred names: %v", err)
			}
		}

		// step 1: read the row's column values
		colValues := getColumnValues(columnNames, columnTypes, rows)

		// step 2: output the column values in RDF format
		blankNodeLabel := tableGuide.blankNodeGenerator.generateBlankNode(tableInfo, colValues)
		m.outputColumnValues(&RowMetaInfo{
			colValues:       colValues,
			columnPredNames: columnPredNames,
			columnNames:     columnNames,
			blankNodeLabel:  blankNodeLabel,
			columnTypes:     columnTypes,
		}, tableInfo)

		// step 3: record mappings to the blankNodeLabel so that future tables can look up the
		// blankNodeLabel
		tableGuide.valuesRecorder.record(tableInfo, colValues, blankNodeLabel)
	}
	return nil
}

func getColumnTypesAndPreNames(rows *sql.Rows, tableGuide *TableGuide,
	tableInfo *TableInfo) ([]string, []*sql.ColumnType, []string, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, nil, err
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, nil, nil, err
	}
	// initialize the columnPredicateNames
	var columnPredicateNames []string
	for _, column := range columns {
		columnPredicateNames = append(columnPredicateNames,
			tableGuide.predNameGenerator.generatePredicateName(tableInfo, column))
	}
	return columns, columnTypes, columnPredicateNames, nil
}

func (m *DumpMeta) outputColumnValues(rowMetaInfo *RowMetaInfo, tableInfo *TableInfo) {
	for i, colValue := range rowMetaInfo.colValues {
		predicate := rowMetaInfo.columnPredNames[i]
		outputPlainCell(rowMetaInfo.blankNodeLabel, rowMetaInfo.columnTypes[i].DatabaseTypeName(),
			predicate,
			colValue, m.dataWriter)
	}

	for _, constraint := range tableInfo.foreignKeyConstraints {
		if len(constraint.parts) == 0 {
			logger.Fatalf("The constraint should have at least one part: %v", constraint)
		}

		foreignTableName := constraint.parts[0].remoteTableName

		refLabel := getRefLabelFromConstraint(rowMetaInfo, tableInfo,
			m.tableInfos[foreignTableName],
			constraint)
		foreignUidLabel := m.tableGuides[foreignTableName].valuesRecorder.getUidLabel(refLabel)
		outputPlainCell(rowMetaInfo.blankNodeLabel, "UID",
			getPredFromConstraint(tableInfo.tableName, SEPERATOR, constraint),
			foreignUidLabel, m.dataWriter)
	}
}

func outputPlainCell(uidLabel string, dbType string, predName string,
	colValue interface{}, writer *bufio.Writer) {
	// step 1: each cell value should be stored under a predicate
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

func getRefLabelFromConstraint(rowMetaInfo *RowMetaInfo,
	tableInfo *TableInfo, foreignTableInfo *TableInfo,
	constraint *ForeignKeyConstraint) string {

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "_:%s%s", foreignTableInfo.tableName, SEPERATOR)

	foreignKeyColumnNames := make(map[string]string)
	for _, part := range constraint.parts {
		foreignKeyColumnNames[part.columnName] = part.remoteColumnName
	}

	foreignColumnIndices := getColumnIndices(tableInfo, func(info *TableInfo, column string) bool {
		_, ok := foreignKeyColumnNames[column]
		return ok
	})

	// replace the column names to be the foreign column names
	for _, columnIdx := range foreignColumnIndices {
		columnIdx.name = foreignKeyColumnNames[columnIdx.name]
	}

	return getAliasLabel(foreignTableInfo.columns, foreignTableInfo.tableName, SEPERATOR,
		foreignColumnIndices,
		rowMetaInfo.colValues)
}
