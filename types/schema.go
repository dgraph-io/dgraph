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
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"

	"github.com/dgraph-io/dgraph/x"
)

// Schema stores the types for all predicates in the system.
var (
	schema = make(map[string]Type)
	sfile  = flag.String("sfile", "../../types/schema.json", "Path to the file that specifies schema in json format")
)

// init function for types package.
func init() {
	x.AddInit(LoadSchema)
}

// LoadSchema loads the schema and checks for errors.
func LoadSchema() {
	file, err := ioutil.ReadFile(*sfile)
	if err != nil {
		log.Fatalf("Schema load error:%v", err)
	}
	s := make(map[string]string)
	if err = json.Unmarshal(file, &s); err != nil {
		log.Fatalf("Schema load error:%v", err)
	}
	// define object entity to denote all entities in the system (subject/object in RDF)
	// In future, entities could be classified further into specific objects like user
	objectType := Object{
		Name:        "Object",
		Description: "Denotes an entity in the system",
		Attributes: map[string]Type{
			"_uid_": idType,
		},
	}
	// go over schema file values and assign appropriate types from type system
	for k, v := range s {
		switch v {
		case "int":
			schema[k] = intType
		case "float":
			schema[k] = floatType
		case "string":
			schema[k] = stringType
		case "bool":
			schema[k] = booleanType
		case "id":
			schema[k] = idType
		case "object":
			schema[k] = objectType
		default:
			log.Fatalf("Unknown type:%v in input schema file for predicate:%v", v, k)
		}
	}
}

// SchemaType fetches types for a predicate from schema map
func SchemaType(p string) Type {
	v, present := schema[p]
	if !present {
		log.Printf("Schema does not have type definition for:%v\n", p)
	}
	return v
}
