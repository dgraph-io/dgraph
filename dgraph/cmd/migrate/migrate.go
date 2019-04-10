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
)

// dumpTable reads data from a table and sends to standard output
func dumpTable(table string, tableGuide *TableGuide, pool *sql.DB) error {
	query := fmt.Sprintf(`select * from %s`, table)
	rows, err := pool.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var columns []string
	var columnTypes []*sql.ColumnType
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
		}

		colValues := make([]interface{}, 0, len(columns))
		for i := 0; i < len(columns); i++ {
			switch columnTypes[i].ScanType().Kind() {
			case reflect.String:
				colValues = append(colValues, new(string))
			case reflect.Int:
				colValues = append(colValues, new(int))
			case reflect.Int32:
				colValues = append(colValues, new(int32))
			default:
				panic(fmt.Sprintf("unknown type: %v", columnTypes[i].ScanType()))
			}
		}
		rows.Scan(colValues...)

		subjectLabel := inferSubjectLabel(table, columns, colValues, tableGuide)

		for i := 0; i < len(columns); i++ {
			// dereference the pointer
			colName := columns[i]
			if colName == tableGuide.UidColumn {
				continue
			}

			fmt.Printf("%s <%s> %v .", subjectLabel,
				tableGuide.ColumnNameToPredicate[colName],
				reflect.ValueOf(colValues[i]).Elem())
		}
		fmt.Println()
	}
	return nil
}
