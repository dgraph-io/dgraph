/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package types

import (
	"flag"
	"os"
	"testing"
)

func createSchemaFile() (*os.File, error) {
	file, err := os.Create("test_schema.json")
	if err != nil {
		return nil, err
	}
	s := `
		{
			"name": "string",
			"age": "int",
			"friend": "object"
		}
	`
	file.WriteString(s)

	return file, nil
}

// TestLoadSchema tests schema reading and parsing from input schema file
func TestLoadSchema(t *testing.T) {
	file, err := createSchemaFile()
	if err != nil {
		t.Error(err)
	}
	defer file.Close()
	defer os.Remove(file.Name())

	// set test schema file path
	flag.Set("sfile", file.Name())

	// load schema from json file
	LoadSchema()
}

// TestSchemaType tests fetching type info from schema map using predicate names
func TestSchemaType(t *testing.T) {
	file, err := createSchemaFile()
	if err != nil {
		t.Error(err)
	}
	defer file.Close()
	defer os.Remove(file.Name())

	// set test schema file path
	flag.Set("sfile", file.Name())

	typ := SchemaType("name")

	if _, ok := typ.(Scalar); !ok {
		t.Error("Type assertion failed for predicate:name")
	}
	typ = SchemaType("friend")
	if _, ok := typ.(Object); !ok {
		t.Error("Type assertion failed for predicate:age")
	}
}
