/*
 * Copyright 2017 Dgraph Labs, Inc.
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
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"
)

func unmarshalToStruct(f reflect.StructField, n *protos.Node, val reflect.Value) error {
	typ := f.Type
	if typ.Kind() == reflect.Slice {
		// Get underlying type. This is sent to unmarshal so that
		// it can unmarshal node and its properties.
		typ = typ.Elem()
	}

	rcv, err := unmarshal(n, typ)
	if err != nil {
		return err
	}

	fieldVal := val.FieldByName(f.Name)
	if !fieldVal.CanSet() {
		return fmt.Errorf("Cant set field: %+v", f.Name)
	}
	// Append if slice else set.
	if f.Type.Kind() == reflect.Slice {
		fieldVal.Set(reflect.Append(fieldVal, rcv))
	} else {
		fieldVal.Set(rcv)
	}
	return nil
}

func unmarshalNode(n *protos.Node, val reflect.Value) error {
	fmap := fieldMap(val.Type())
	attr := strings.ToLower(n.Attribute)
	field, ok := fmap[attr]
	if !ok {
		return nil
	}
	if n.Attribute == "@facets" {
		// We handle facets separately, so that they can be unmarshalled
		// at the same level as other properties.
		for _, cn := range n.Children {
			attr := cn.Attribute
			if attr == "_" {
				// _ has properties in a child node.
				if len(cn.Children) != 1 {
					continue
				}
				if field, ok := fmap["@facets"]; ok {
					if err := unmarshalToStruct(field, cn.Children[0],
						val); err != nil {
						return err
					}
				}
			} else {
				// Other facets should have a dgraph tag attribute@facets.
				key := fmt.Sprintf("%s@facets", attr)
				if field, ok := fmap[key]; ok {
					if err := unmarshalToStruct(field, cn,
						val); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := unmarshalToStruct(field, n, val); err != nil {
		return err
	}
	if val.Kind() == reflect.Ptr {
		val = reflect.New(val.Type().Elem()).Elem()
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
	case reflect.String:
		switch value.Val.(type) {
		case *protos.Value_GeoVal:
			v := value.GetGeoVal()
			if len(v) == 0 {
				return nil
			}

			g, err := wkb.Unmarshal(v)
			if err != nil {
				return nil
			}
			b, err := geojson.Marshal(g)
			if err != nil {
				return nil
			}
			f.SetString(string(b))
		default:
			f.SetString(value.GetStrVal())
		}
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
			if value == nil {
				return nil
			}
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
		default:
			// We would be here when an attr has multiple values.
			// TODO - Refactor to remove this duplication.
			elemType := field.Type.Elem()
			switch elemType.Kind() {
			case reflect.String:
				f.Set(reflect.Append(f, reflect.ValueOf(value.GetStrVal())))
			case reflect.Int64, reflect.Int:
				f.Set(reflect.Append(f, reflect.ValueOf(value.GetIntVal())))
			case reflect.Float64:
				f.Set(reflect.Append(f, reflect.ValueOf(value.GetDoubleVal())))
			case reflect.Bool:
				f.Set(reflect.Append(f, reflect.ValueOf(value.GetBoolVal())))
			case reflect.Uint64:
				f.Set(reflect.Append(f, reflect.ValueOf(value.GetUidVal())))
			case reflect.Struct:
				switch elemType {
				case reflect.TypeOf(time.Time{}):
					v := value.GetStrVal()
					if v == "" {
						return nil
					}
					t, err := time.Parse(time.RFC3339, v)
					if err == nil {
						f.Set(reflect.Append(f, reflect.ValueOf(t)))
					}
				default:
				}
			}
		}
	default:
	}
	return nil
}

func unmarshal(n *protos.Node, typ reflect.Type) (reflect.Value, error) {
	var val reflect.Value
	if typ.Kind() == reflect.Array {
		return val, fmt.Errorf("Arrays are not supported. Try using a slice.")
	}

	fmap := fieldMap(typ)
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
		if err := unmarshalNode(child, val); err != nil {
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

func resetStruct(t reflect.Type, v reflect.Value) {
	for i := 0; i < v.NumField() && i < t.NumField(); i++ {
		f := v.Field(i)
		ft := t.Field(i)
		if !f.CanSet() {
			continue
		}
		switch ft.Type.Kind() {
		case reflect.Slice:
			f.Set(reflect.MakeSlice(ft.Type, 0, 0))
		case reflect.Struct:
			resetStruct(ft.Type, f)
		case reflect.Ptr:
			if ft.Type.Elem().Kind() != reflect.Struct {
				continue
			}
			fv := reflect.New(ft.Type.Elem())
			resetStruct(ft.Type.Elem(), fv.Elem())
			f.Set(fv)
		default:
		}
	}
}

// Unmarshal is used to unpack a query response into a custom struct.  The
// response from Dgraph.Run (a *protos.Response) has 4 fields, L(Latency),
// Schema, AssignedUids and N(Nodes). This function takes in the nodes part of
// the response and tries to unmarshal it into the given struct v.
//
// protos.Response.N is a slice of Nodes, one for each named query block in the
// request.  Each node in that slice has attribute "_root_" and a child for
// each node returned as a result by that query block.  For a response resp,
// and struct variable v, with a field tagged with the same name as a query
// block:
// Unmarshal(resp.N, s)
// will try to match named query blocks with tags in s and then unmarshall the
// the matched block into the matched fields of s.
//
// Unmarshal does not have to be called at resp.N.  Clients can navigate to a
// particular part of the response and unmarshal the children.
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

	// This is important so that all fields of the struct given by the user are reset.
	resetStruct(val.Type(), val)

	// Root can have multiple query blocks.
	for _, node := range n {
		for _, child := range node.Children {
			if err := unmarshalNode(child, val); err != nil {
				return err
			}
		}
	}
	return nil
}
