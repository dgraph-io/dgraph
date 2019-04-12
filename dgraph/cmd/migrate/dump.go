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
	"reflect"
	"sort"
	"strings"

	"github.com/spf13/viper"
)

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

// dumpTable reads data from a table and sends to standard output
func dumpTable(table string, tableInfo *TableInfo, tableGuide *TableGuide, pool *sql.DB) error {
	sortedColumns := getSortedColumns(tableInfo)
	columnsQuery := strings.Join(sortedColumns, ",")
	query := fmt.Sprintf(`select %s from %s`, columnsQuery, table)
	fmt.Printf("query %s\n", query)
	rows, err := pool.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var columns []string
	var columnTypes []*sql.ColumnType
	var columnPredicateNames []string
	for rows.Next() {
		if columns == nil {
			columns, err = rows.Columns()
			if err != nil {
				return err
			}

			columnTypes, err = rows.ColumnTypes()
			if err != nil {
				return err
			}

			// initialize the columnPredicateNames
			for _, column := range columns {
				columnPredicateNames = append(columnPredicateNames,
					tableGuide.predNameGenerator.generatePredicateName(tableInfo, column))
			}
		}

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

		colValues := getColValues(colValuePtrs)

		uidLabel := tableGuide.keyGenerator.generateKey(tableInfo, colValues)
		for i, colValue := range colValues {
			fmt.Printf("%s %s ", uidLabel, columnPredicateNames[i])
			if columnTypes[i].DatabaseTypeName() == "VARCHAR" {
				fmt.Printf("%q . \n", colValue)
			} else {
				fmt.Printf("%v . \n", colValue)
			}
		}
	}
	return nil
}

func getColValues(colValuePtrs []interface{}) []interface{} {
	colValues := make([]interface{}, 0, len(colValuePtrs))
	for _, colValuePtr := range colValuePtrs {
		// dereference the pointer to get the actual value
		colValues = append(colValues, reflect.ValueOf(colValuePtr).Elem())
	}
	return colValues
}

func run(conf *viper.Viper) error {
	mysqlUser := conf.GetString("mysql_user")
	mysqlDB := conf.GetString("mysql_db")
	mysqlPassword := conf.GetString("mysql_password")
	mysqlTables := conf.GetString("mysql_tables")
	//outputFile := conf.GetString("output")

	if len(mysqlUser) == 0 {
		logger.Fatalf("the mysql_user property should not be empty")
	}
	if len(mysqlDB) == 0 {
		logger.Fatalf("the mysql_db property should not be empty")
	}
	if len(mysqlPassword) == 0 {
		logger.Fatalf("the mysql_password property should not be empty")
	}
	/*
		if len(outputFile) == 0 {
			logger.Fatalf("the output file should not be empty")
		}
	*/

	/*
		output, err := os.OpenFile(outputFile, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		defer output.Close()
		ioWriter := bufio.NewWriter(output)
	*/

	pool, cancelFunc, err := getMySQLPool(mysqlUser, mysqlDB, mysqlPassword)
	if err != nil {
		return err
	}
	defer cancelFunc()

	tablesToRead, err := readMySqlTables(mysqlTables, pool)
	if err != nil {
		return err
	}

	tables := make(map[string]*TableInfo, 0)
	for _, table := range tablesToRead {
		tableInfo, err := getTableInfo(table, pool)
		if err != nil {
			return err
		}
		tables[tableInfo.tableName] = tableInfo
	}
	populateReferencedByColumns(tables)

	tableGuides := genGuide(tables)

	tablesSorted, err := topoSortTables(tables)
	if err != nil {
		return err
	}

	//fmt.Printf("topo sorted tables:\n")
	for _, table := range tablesSorted {
		fmt.Printf("%s\n", table)
		guide := tableGuides[table]
		//spew.Dump(guide.indexGenerator.generateDgraphIndices(tables[table]))
		dumpTable(table, tables[table], guide, pool)
	}

	return nil
}
