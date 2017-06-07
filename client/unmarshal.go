/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package client

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/dgraph-io/dgraph/protos"
)

func unmarshal(n *protos.Node, typ reflect.Type) (reflect.Value, error) {
	// TODO - Return error if struct and property response types don't match.
	fmap := fieldMap(typ)
	val := reflect.New(typ).Elem()

	for _, prop := range n.Properties {
		if field, ok := fmap[strings.ToLower(prop.Prop)]; ok {
			switch field.Type.Kind() {
			// TODO - Handle all types.
			case reflect.String:
				val.FieldByName(field.Name).SetString(prop.Value.GetStrVal())
			case reflect.Int:
				val.FieldByName(field.Name).SetInt(prop.Value.GetIntVal())
			case reflect.Float64:
				val.FieldByName(field.Name).SetFloat(prop.Value.GetDoubleVal())
			default:
			}
		}
	}
	for _, child := range n.Children {
		attr := child.Attribute
		if field, ok := fmap[strings.ToLower(attr)]; ok {
			fieldVal := val.FieldByName(field.Name)
			ftyp := field.Type
			// TODO - Also include array and other types
			typ := ftyp
			if typ.Kind() == reflect.Slice {
				// Get underlying type.
				typ = typ.Elem()
			}

			rcv, err := unmarshal(child, typ)
			if err != nil {
				return val, err
			}

			if ftyp.Kind() == reflect.Slice {
				fieldVal.Set(reflect.Append(fieldVal, rcv))
			} else {
				fieldVal.Set(rcv)
			}

		}
	}
	return val, nil
}

// TODO - Handle pointer type
func fieldMap(typ reflect.Type) map[string]reflect.StructField {
	// Map[tag/fieldName] => StructField
	fmap := make(map[string]reflect.StructField)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		// If dgraph tag for a field exists we store that.
		if tag, ok := field.Tag.Lookup("dgraph"); ok {
			// Store in lower case to do a case-insensitive match.
			fmap[strings.ToLower(tag)] = field
		} else {
			// Else we store the field Name.
			fmap[strings.ToLower(field.Name)] = field
		}
	}
	return fmap
}

func Unmarshal(n []*protos.Node, v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("Unmarshal error(non-pointer: %+v)", reflect.TypeOf(v))
	}
	val := rv.Elem()
	fmap := fieldMap(val.Type())

	// Root can have multiple query blocks.
	for _, node := range n {
		for _, child := range node.Children {
			// TODO - Move this out to a function so that unmarshal can share it.
			attr := strings.ToLower(child.Attribute)
			field, ok := fmap[attr]
			if !ok {
				continue
			}

			ftyp := field.Type
			// TODO - Also include array and other types
			typ := ftyp
			if typ.Kind() == reflect.Slice {
				// Get underlying type.
				typ = typ.Elem()
			}

			rcv, err := unmarshal(child, typ)
			if err != nil {
				return err
			}

			fieldVal := val.FieldByName(field.Name)
			if ftyp.Kind() == reflect.Slice {
				fieldVal.Set(reflect.Append(fieldVal, rcv))
			} else {
				fieldVal.Set(rcv)
			}
		}
	}
	return nil
}
