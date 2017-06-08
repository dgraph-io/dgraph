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
	"time"

	"github.com/dgraph-io/dgraph/protos"
)

func unmarshalChild(n *protos.Node, val reflect.Value) error {
	fmap := fieldMap(val.Type())
	attr := strings.ToLower(n.Attribute)
	field, ok := fmap[attr]
	if !ok {
		return nil
	}

	ftyp := field.Type
	typ := ftyp
	if typ.Kind() == reflect.Slice {
		// Get underlying type.
		typ = typ.Elem()
	}

	rcv, err := unmarshal(n, typ)
	if err != nil {
		return err
	}

	if val.Kind() == reflect.Ptr {
		val = reflect.New(val.Type().Elem()).Elem()
	}

	fieldVal := val.FieldByName(field.Name)
	if !fieldVal.CanSet() {
		return fmt.Errorf("Cant set field: %+v", field.Name)
	}

	if ftyp.Kind() == reflect.Slice {
		fieldVal.Set(reflect.Append(fieldVal, rcv))
	} else {
		fieldVal.Set(rcv)
	}

	return nil
}

func setField(val reflect.Value, value *protos.Value, field reflect.StructField) error {
	// Cannnot unmarshal into unexported fields.
	if len(field.PkgPath) > 0 {
		return nil
	}

	if val.Kind() == reflect.Ptr && val.IsNil() {
		val = reflect.New(val.Type().Elem()).Elem()
	}

	if val.Kind() != reflect.Struct {
		return fmt.Errorf("Can only set fields for struct types. Got: %+v", val.Kind())
	}

	f := val.FieldByName(field.Name)
	if !f.CanSet() {
		return fmt.Errorf("Cant set field: %+v", field.Name)
	}

	switch field.Type.Kind() {
	// TODO - Handle all types.
	case reflect.String:
		f.SetString(value.GetStrVal())
	case reflect.Int64, reflect.Int:
		f.SetInt(value.GetIntVal())
	case reflect.Float64:
		f.SetFloat(value.GetDoubleVal())
	case reflect.Bool:
		f.SetBool(value.GetBoolVal())
	case reflect.Uint64:
		f.SetUint(value.GetUidVal())
	case reflect.Struct:
		switch field.Type {
		case reflect.TypeOf(time.Time{}):
			v := value.GetStrVal()
			if v == "" {
				return nil
			}

			t, err := time.Parse(time.RFC3339, v)
			if err == nil {
				f.Set(reflect.ValueOf(t))
			}

		}
	case reflect.Slice:
		switch field.Type {
		case reflect.TypeOf([]byte{}):
			switch value.Val.(type) {
			case *protos.Value_GeoVal:
				v := value.GetGeoVal()
				if len(v) == 0 {
					return nil
				}

				f.Set(reflect.ValueOf(v))
			case *protos.Value_BytesVal:
				v := value.GetBytesVal()
				f.Set(reflect.ValueOf(v))
			}
		}

	default:
	}

	return nil
}

func unmarshal(n *protos.Node, typ reflect.Type) (reflect.Value, error) {
	fmap := fieldMap(typ)

	var val reflect.Value
	if typ.Kind() == reflect.Ptr {
		// We got a pointer, lets set val to a settable type.
		val = reflect.New(typ.Elem()).Elem()
	} else {
		val = reflect.New(typ).Elem()
	}

	for _, prop := range n.Properties {
		if field, ok := fmap[strings.ToLower(prop.Prop)]; ok {
			if err := setField(val, prop.Value, field); err != nil {
				return val, err
			}

		}
	}
	for _, child := range n.Children {
		if err := unmarshalChild(child, val); err != nil {
			return val, err
		}
	}

	if typ.Kind() == reflect.Ptr {
		// Lets convert val back to a pointer.
		val = val.Addr()
	}

	return val, nil
}

func fieldMap(typ reflect.Type) map[string]reflect.StructField {
	// We cant do NumField on non-struct types.
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return nil
	}

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
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("Cannot unmarshal into: %v . Require a pointer to a struct",
			val.Kind())
	}

	// Root can have multiple query blocks.
	for _, node := range n {
		for _, child := range node.Children {
			if err := unmarshalChild(child, val); err != nil {
				return err
			}
		}
	}
	return nil
}
