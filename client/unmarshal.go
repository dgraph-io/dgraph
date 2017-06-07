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
	"github.com/dgraph-io/dgraph/x"
)

func unmarshal(n *protos.Node, typ reflect.Type) (reflect.Value, error) {
	// TODO - Return error if struct and property response types don't match.
	m := buildTypeMap(typ)
	val := reflect.New(typ).Elem()
	for _, prop := range n.Properties {
		if field, ok := m[strings.ToLower(prop.Prop)]; ok {
			f := val.FieldByName(field.Name)
			if !f.IsValid() {
				fmt.Println("Invalid field", prop.Prop, field.Name)
			}
			switch field.Type {
			case reflect.TypeOf(""):
				val.FieldByName(field.Name).SetString(prop.Value.GetStrVal())
			case reflect.TypeOf(0):
				val.FieldByName(field.Name).SetInt(prop.Value.GetIntVal())
			default:
				x.Fatalf("Unhandled tag")
			}
		}
	}
	for _, c := range n.Children {
		if field, ok := m[strings.ToLower(c.Attribute)]; ok {
			f := val.FieldByName(field.Name)
			if !f.IsValid() {
				fmt.Println("Invalid field", c.Attribute, field.Name)
			}
			typ := field.Type
			// TODO - Also include array and other types
			elemType := typ
			if typ.Kind() == reflect.Slice {
				elemType = typ.Elem()
			}
			//			if typ.Kind() == reflect.Ptr {
			//				typ = elem.Elem().Type()
			//				fmt.Println("yo type", typ)
			//			}
			valChild, err := unmarshal(c, elemType)
			if err != nil {
				return val, err
			}
			if typ.Kind() == reflect.Slice {
				f.Set(reflect.Append(f, valChild))
			} else {
				f.Set(valChild)
			}

		}
	}
	//fmt.Println("val", val)
	return val, nil
}

// TODO - Handle pointer type
func buildTypeMap(typ reflect.Type) map[string]reflect.StructField {
	tagTypeMap := make(map[string]reflect.StructField)
	//	// Return error if nil
	//	if val.Kind() == reflect.Ptr {
	//		val = val.Elem()
	//	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if tag, ok := field.Tag.Lookup("dgraph"); ok {
			// We want to do a case-insensitive match.
			tagTypeMap[strings.ToLower(tag)] = field
		} else {
			tagTypeMap[strings.ToLower(field.Name)] = field
			// Store field name lower case and match against that.
		}
	}
	return tagTypeMap
}

func Unmarshal(n []*protos.Node, v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("Unmarshal error(non-pointer: %+v)", reflect.TypeOf(v))
	}
	val := rv.Elem()
	tagTypeMap := buildTypeMap(val.Type())

	for _, node := range n {
		for _, child := range node.Children {
			// We want to do a case-insensitive match.
			nodeAttr := strings.ToLower(child.Attribute)
			field, ok := tagTypeMap[nodeAttr]
			if !ok {
				continue
			}
			typ := field.Type
			// TODO - Also include array and other types
			elemType := typ
			if typ.Kind() == reflect.Slice {
				elemType = typ.Elem()
			}
			//			if typ.Kind() == reflect.Ptr {
			//				typ = elem.Elem().Type()
			//				fmt.Println("yo type", typ)
			//			}
			valChild, err := unmarshal(child, elemType)
			if err != nil {
				return err
			}
			f := val.FieldByName(field.Name)
			if typ.Kind() == reflect.Slice {
				f.Set(reflect.Append(f, valChild))
			} else {
				f.Set(valChild)
			}
			// if typ is slice parse type and pass that.

		}
	}
	return nil
}
