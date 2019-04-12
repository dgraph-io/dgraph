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

const (
	UNKNOWN DataType = iota
	INT
	STRING
	FLOAT
	DOUBLE
	DATETIME
	UID // foreign key reference, which would corrspond to uid type in Dgraph
)

var typeToString map[DataType]string
var mysqlTypePrefixToGoType map[string]DataType

func init() {
	typeToString = make(map[DataType]string)
	typeToString[UNKNOWN] = "unknown"
	typeToString[INT] = "int"
	typeToString[STRING] = "string"
	typeToString[FLOAT] = "float"
	typeToString[DOUBLE] = "double"
	typeToString[DATETIME] = "datetime"
	typeToString[UID] = "uid"

	mysqlTypePrefixToGoType = make(map[string]DataType)
	mysqlTypePrefixToGoType["int"] = INT
	mysqlTypePrefixToGoType["varchar"] = STRING
	mysqlTypePrefixToGoType["date"] = DATETIME
	mysqlTypePrefixToGoType["time"] = DATETIME
	mysqlTypePrefixToGoType["datetime"] = DATETIME
	mysqlTypePrefixToGoType["float"] = FLOAT
	mysqlTypePrefixToGoType["double"] = DOUBLE
}

func (t DataType) String() string {
	return typeToString[t]
}
