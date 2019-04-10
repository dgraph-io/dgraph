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
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"github.com/spf13/viper"
)

type TableGuide struct {
	TableName string

	// optionally one column can be used as the uidColumn, whose values are mapped to Dgraph uids
	UidColumn string

	// mappedPredNames[i] stores the predicate name for value in column i of a MySQL table
	ColumnNameToPredicate map[string]string

	// the current table row number, used to generate labels for uids
	TableRowNum int
}

func genGuideForTable(table string, pool *sql.DB) (*TableGuide, error) {
	query := fmt.Sprintf(`describe %s`, table)
	rows, err := pool.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 6 columns will be returned for the describe command
	const PROPERTIES = 6
	// Field, Type, Null, Key, Default, Extra
	columnProperties := make([]interface{}, 0, PROPERTIES)
	for i := 0; i < PROPERTIES; i++ {
		columnProperties = append(columnProperties, new(string))
	}

	columnNameToPredicate := make(map[string]string)
	columnNames := make([]string, 0)
	for rows.Next() {
		rows.Scan(columnProperties...)
		columnName := reflect.ValueOf(columnProperties[0]).Elem().String()
		columnNameToPredicate[columnName] = ""
		columnNames = append(columnNames, columnName)
	}
	fmt.Printf("Columns in the table %s:\n", table)
	for _, colName := range columnNames {
		fmt.Printf("%s\n", colName)
	}

	tableGuide := &TableGuide{
		TableName: table,
	}
	tableGuide.UidColumn, err = askUidColumn(columnNameToPredicate)
	if err != nil {
		return nil, err
	}

	for _, colName := range columnNames {
		if colName == tableGuide.UidColumn {
			continue
		}
		columnNameToPredicate[colName], err = askPredicateForColumn(colName)
		if err != nil {
			return nil, err
		}
	}

	tableGuide.ColumnNameToPredicate = columnNameToPredicate
	return tableGuide, nil
}

func genGuide(conf *viper.Viper) error {
	mysqlUser := conf.GetString("mysql_user")
	mysqlDB := conf.GetString("mysql_db")
	mysqlPassword := conf.GetString("mysql_password")
	mysqlTables := conf.GetString("mysql_tables")
	outputFile := conf.GetString("output")

	if len(mysqlUser) == 0 {
		logger.Fatalf("the mysql_user property should not be empty")
	}
	if len(mysqlDB) == 0 {
		logger.Fatalf("the mysql_db property should not be empty")
	}
	if len(mysqlPassword) == 0 {
		logger.Fatalf("the mysql_password property should not be empty")
	}
	if len(outputFile) == 0 {
		logger.Fatalf("the output file should not be empty")
	}

	output, err := os.OpenFile(outputFile, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer output.Close()
	ioWriter := bufio.NewWriter(output)

	pool, cancelFunc, err := getMySQLPool(mysqlUser, mysqlDB, mysqlPassword)
	if err != nil {
		return err
	}
	defer cancelFunc()

	tablesToRead, err := readMySqlTables(mysqlTables, pool)
	if err != nil {
		return err
	}

	for _, table := range tablesToRead {
		tableGuide, err := genGuideForTable(table, pool)
		if err != nil {
			return err
		}

		guideBytes, err := json.Marshal(tableGuide)
		if err != nil {
			return err
		}

		if _, err = ioWriter.Write(guideBytes); err != nil {
			return err
		}
	}
	ioWriter.Flush()
	return nil
}
