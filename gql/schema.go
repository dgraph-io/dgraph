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

package gql

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/dgraph-io/dgraph/types"
)

// schema stores the types for all predicates in the system.
var schema = make(map[string]types.Type)

// LoadSchema loads the schema and checks for errors.
func LoadSchema(fileName string) error {
	file, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	s := make(map[string]string)
	if err = json.Unmarshal(file, &s); err != nil {
		return err
	}
	// go over schema file values and assign appropriate types from type system
	for k, v := range s {
		t := types.TypeForName(v)
		if t == nil {
			return fmt.Errorf("Unknown type:%v in input schema file for predicate:%v", v, k)
		}
		schema[k] = t
	}
	return nil
}

// SchemaType fetches types for a predicate from schema map
func SchemaType(p string) types.Type {
	return schema[p]
}
