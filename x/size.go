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

package x

import (
	"reflect"
	"strings"
	"log"
)

func DeepSizeOf(v interface{}) uintptr {
	log.Printf("v %+v", v)
	var sz uintptr
	val := reflect.ValueOf(v)
	log.Printf("kind %v\n", val.Kind())
	switch val.Kind() {
	case reflect.String:
		sz += val.Type().Size()
		log.Printf("len %v\n", len(val.String()))
		sz += uintptr(len(val.String()))
	case reflect.Array:
		for i := 0; i < val.Len(); i++ {
			sz += DeepSizeOf(val.Index(i))
		}
	case reflect.Slice:
		if val.IsNil() {
			break
		}
		for i := 0; i < val.Len(); i++ {
			sz += DeepSizeOf(val.Index(i))
		}
	case reflect.Interface:
		if val.IsNil() {
			break
		}
		sz += DeepSizeOf(val.Elem())
	case reflect.Ptr:
		sz += val.Type().Size()
		if val.IsNil() {
			break
		}
		log.Printf("elem ptr %+v", val.Elem())
		sz += DeepSizeOf(val.Elem())
	case reflect.Struct:
		for i, n := 0, val.NumField(); i < n; i++ {
			fieldName := val.Type().Field(i).Name
			if len(fieldName) == 0 || string(fieldName[0]) ==
				strings.ToLower(string(fieldName[0])) {
				continue
			}
			log.Printf("kind field %v", val.Field(i).Kind())
			sz += DeepSizeOf(val.Field(i).Interface())
		}
	case reflect.Map:
		if val.IsNil() {
			break
		}
		for _, k := range val.MapKeys() {
			mapVal := val.MapIndex(k)
			sz += k.Type().Size() + DeepSizeOf(mapVal)
		}
	default:
		sz += val.Type().Size()
	}

	return sz
}
