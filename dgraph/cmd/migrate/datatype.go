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
	unknownType dataType = iota
	intType
	stringType
	floatType
	doubleType
	datetimeType
	uidType // foreign key reference, which would corrspond to uid type in Dgraph
)

// the typeToString map is used to generate the Dgraph schema file
var typeToString map[dataType]string

// the sqlTypeToInternal map is used to parse date types in SQL schema
var sqlTypeToInternal map[string]dataType

func initDataTypes() {
	typeToString = make(map[dataType]string)
	typeToString[unknownType] = "unknown"
	typeToString[intType] = "int"
	typeToString[stringType] = "string"
	typeToString[floatType] = "float"
	typeToString[doubleType] = "double"
	typeToString[datetimeType] = "datetime"
	typeToString[uidType] = "uid"

	sqlTypeToInternal = make(map[string]dataType)
	sqlTypeToInternal["int"] = intType
	sqlTypeToInternal["varchar"] = stringType
	sqlTypeToInternal["text"] = stringType
	sqlTypeToInternal["date"] = datetimeType
	sqlTypeToInternal["time"] = datetimeType
	sqlTypeToInternal["datetime"] = datetimeType
	sqlTypeToInternal["float"] = floatType
	sqlTypeToInternal["double"] = doubleType
	sqlTypeToInternal["decimal"] = floatType
}

func (t dataType) String() string {
	return typeToString[t]
}
