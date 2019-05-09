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

const (
	UNKNOWN DataType = iota
	INT
	STRING
	FLOAT
	DOUBLE
	DATETIME
	UID // foreign key reference, which would corrspond to uid type in Dgraph
)

// the typeToString map is used to generate the Dgraph schema file
var typeToString map[DataType]string

// the sqlTypeToInternal map is used to parse date types in SQL schema
var sqlTypeToInternal map[string]DataType

func initDataTypes() {
	typeToString = make(map[DataType]string)
	typeToString[UNKNOWN] = "unknown"
	typeToString[INT] = "int"
	typeToString[STRING] = "string"
	typeToString[FLOAT] = "float"
	typeToString[DOUBLE] = "double"
	typeToString[DATETIME] = "datetime"
	typeToString[UID] = "uid"

	sqlTypeToInternal = make(map[string]DataType)
	sqlTypeToInternal["int"] = INT
	sqlTypeToInternal["varchar"] = STRING
	sqlTypeToInternal["text"] = STRING
	sqlTypeToInternal["date"] = DATETIME
	sqlTypeToInternal["time"] = DATETIME
	sqlTypeToInternal["datetime"] = DATETIME
	sqlTypeToInternal["float"] = FLOAT
	sqlTypeToInternal["double"] = DOUBLE
	sqlTypeToInternal["decimal"] = FLOAT
}

func (t DataType) String() string {
	return typeToString[t]
}
