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
	"os"
	"sort"
	"strings"

	"github.com/spf13/viper"
)

// getSortedColumns sorts the column alphabetically using the column names
// and return the column names as a slice
func getSortedColumns(tableInfo *TableInfo) []string {
	columns := make([]string, 0)
	for column := range tableInfo.columns {
		columns = append(columns, column)
	}
	sort.Slice(columns, func(i, j int) bool {
		return columns[i] < columns[j]
	})
	return columns
}

type RowMetaInfo struct {
	colValues       []interface{}
	columnPredNames []string
	columnNames     []string
	blankNodeLabel  string
	columnTypes     []*sql.ColumnType
}

// dumpTable reads data from a table and sends to the writer
func dumpTable(table string, tableInfo *TableInfo, tableGuides map[string]*TableGuide,
	pool *sql.DB, writer *bufio.Writer) error {
	tableGuide := tableGuides[table]
	sortedColumns := getSortedColumns(tableInfo)
	columnsQuery := strings.Join(sortedColumns, ",")
	query := fmt.Sprintf(`select %s from %s`, columnsQuery, table)
	rows, err := pool.Query(query)
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
		blankNodeLabel := tableGuide.keyGenerator.generateKey(tableInfo, colValues)
		outputColumnValues(&RowMetaInfo{
			colValues:       colValues,
			columnPredNames: columnPredNames,
			columnNames:     columnNames,
			blankNodeLabel:  blankNodeLabel,
			columnTypes:     columnTypes,
		}, writer, tableInfo, tableGuides)

		// step 3: record mappings to the blankNodeLabel so that future tables can look up the
		// blankNodeLabel
		tableGuide.valuesRecordor.record(tableInfo, colValues, blankNodeLabel)
	}
	return nil
}

func outputColumnValues(rowMetaInfo *RowMetaInfo, writer *bufio.Writer, tableInfo *TableInfo,
	tableGuides map[string]*TableGuide) {
	for i, colValue := range rowMetaInfo.colValues {
		predicate := rowMetaInfo.columnPredNames[i]
		outputPlainCell(rowMetaInfo.blankNodeLabel, rowMetaInfo.columnTypes[i].DatabaseTypeName(),
			predicate,
			colValue, writer)

		for _, constraint := range tableInfo.foreignKeyConstraints {
			if len(constraint.parts) == 0 {
				logger.Fatalf("The constraint should have at least one part: %v", constraint)
			}

			foreignTableName := constraint.parts[0].remoteTableName

			refLabel := getRefLabelFromConstraint(rowMetaInfo, tableInfo, foreignTableName,
				constraint)
			foreignUidLabel := tableGuides[foreignTableName].valuesRecordor.getUidLabel(refLabel)
			outputPlainCell(rowMetaInfo.blankNodeLabel, "UID", getLinkPredicate(predicate), foreignUidLabel,
				writer)
		}
	}
}

func getRefLabelFromConstraint(rowMetaInfo *RowMetaInfo, tableInfo *TableInfo,
	foreignTableName string, constraint *ForeignKeyConstraint) string {

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "_:%s%s", foreignTableName, SEPERATOR)

	foreignKeyColumnNames := make(map[string]interface{})
	for _, part := range constraint.parts {
		foreignKeyColumnNames[part.columnName] = struct{}{}
	}

	foreignKeyColumns := getColumnIndices(tableInfo, func(info *TableInfo, column string) bool {
		_, ok := foreignKeyColumnNames[column]
		return ok
	})

	for _, c := range foreignKeyColumns {
		fmt.Fprintf(&buf, "%s%s%s", c.name, SEPERATOR, rowMetaInfo.colValues[c.index])
	}
	return buf.String()
}

func getColumnValues(columns []string, columnTypes []*sql.ColumnType, rows *sql.Rows) []interface{} {
	colValuePtrs := make([]interface{}, 0, len(columns))
	for i := 0; i < len(columns); i++ {
		switch columnTypes[i].DatabaseTypeName() {
		case "VARCHAR":
			colValuePtrs = append(colValuePtrs, new(string))
		case "INT":
			colValuePtrs = append(colValuePtrs, new(int))
		case "FLOAT":
			colValuePtrs = append(colValuePtrs, new(float64))
		default:
			panic(fmt.Sprintf("unknown type %v at index %d",
				columnTypes[i].ScanType().Kind(), i))
		}
	}
	rows.Scan(colValuePtrs...)
	colValues := ptrToValues(colValuePtrs)
	return colValues
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

func getFileWriter(filename string) (*bufio.Writer, func(), error) {
	output, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, nil, err
	}

	return bufio.NewWriter(output), func() { output.Close() }, nil
}

func run(conf *viper.Viper) error {
	mysqlUser := conf.GetString("mysql_user")
	mysqlDB := conf.GetString("mysql_db")
	mysqlPassword := conf.GetString("mysql_password")
	mysqlTables := conf.GetString("mysql_tables")
	schemaOutput := conf.GetString("output_schema")
	dataOutput := conf.GetString("output_data")

	if len(mysqlUser) == 0 {
		logger.Fatalf("the mysql_user property should not be empty")
	}
	if len(mysqlDB) == 0 {
		logger.Fatalf("the mysql_db property should not be empty")
	}
	if len(mysqlPassword) == 0 {
		logger.Fatalf("the mysql_password property should not be empty")
	}
	if len(schemaOutput) == 0 {
		logger.Fatalf("the schema output file should not be empty")
	}
	if len(dataOutput) == 0 {
		logger.Fatalf("the data output file should not be empty")
	}

	pool, cancelFunc, err := getMySQLPool(mysqlUser, mysqlDB, mysqlPassword)
	if err != nil {
		return err
	}
	defer cancelFunc()

	tablesToRead, err := readMySqlTables(mysqlTables, pool)
	if err != nil {
		return err
	}

	tableInfos := make(map[string]*TableInfo, 0)
	for _, table := range tablesToRead {
		tableInfo, err := getTableInfo(table, pool)
		if err != nil {
			return err
		}
		tableInfos[tableInfo.tableName] = tableInfo
	}
	populateReferencedByColumns(tableInfos)

	tableGuides := getTableGuides(tableInfos)

	return generateSchemaAndData(schemaOutput, dataOutput, tableInfos, tableGuides, pool)
}

func generateSchemaAndData(schemaOutput string, dataOutput string,
	tableInfos map[string]*TableInfo, tableGuides map[string]*TableGuide, pool *sql.DB) error {
	schemaWriter, schemaCancelFunc, err := getFileWriter(schemaOutput)
	if err != nil {
		return err
	}
	defer schemaCancelFunc()
	dataWriter, dataCancelFunc, err := getFileWriter(dataOutput)
	if err != nil {
		return err
	}
	defer dataCancelFunc()
	if err := dumpSchema(tableInfos, tableGuides, schemaWriter); err != nil {
		return fmt.Errorf("error while writing schema file: %v", err)
	}
	if err := dumpTables(tableInfos, tableGuides, pool, dataWriter); err != nil {
		return fmt.Errorf("error while writeng data file: %v", err)
	}
	return nil
}

func dumpSchema(tableInfos map[string]*TableInfo, guides map[string]*TableGuide,
	writer *bufio.Writer) error {
	for table, guide := range guides {
		tableInfo := tableInfos[table]
		for _, index := range guide.indexGenerator.generateDgraphIndices(tableInfo) {
			_, err := writer.WriteString(index)
			if err != nil {
				return fmt.Errorf("error while writing schema: %v", err)
			}
		}
	}
	return writer.Flush()
}

func dumpTables(tableInfos map[string]*TableInfo, tableGuides map[string]*TableGuide,
	pool *sql.DB, writer *bufio.Writer) error {
	tablesSorted, err := topoSortTables(tableInfos)
	if err != nil {
		return err
	}

	for _, table := range tablesSorted {
		fmt.Printf("Dumping table %s\n", table)
		dumpTable(table, tableInfos[table], tableGuides, pool, writer)
	}

	return writer.Flush()
}
