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
	"encoding/binary"
	"encoding/json"
	"math"
	"strconv"

	"github.com/dgraph-io/dgraph/x"
)

// added suffix 'type' to names to distinguish from Go types 'int' and 'string'
var (
	// Int scalar type.
	intType = Scalar{
		Name: "Int",
		Description: "The 'Int' scalar type represents non-fractional signed whole" +
			" numeric values. Int can represent values between -(2^31)" +
			" and 2^31 - 1.",
		NewValue: newInt32Val,
	}
	// Float scalar type.
	floatType = Scalar{
		Name: "Float",
		Description: "The 'Float' scalar type represents signed double-precision" +
			" fractional values	as specified by [IEEE 754]" +
			" (http://en.wikipedia.org/wiki/IEEE_floating_point).",
		NewValue: newFloatVal,
	}
	// String scalar type.
	stringType = Scalar{
		Name: "String",
		Description: "The 'String' scalar type represents textual data, represented" +
			" as UTF-8 character sequences. The String type is most often" +
			" used by GraphQL to represent free-form human-readable text.",
		NewValue: newStringVal,
	}
	// Boolean scalar type.
	booleanType = Scalar{
		Name:        "Boolean",
		Description: "The 'Boolean' scalar type represents 'true' or 'false'.",
		NewValue:    newBoolVal,
	}
	// ID scalar type.
	idType = Scalar{
		Name: "ID",
		Description: "The 'ID' scalar type represents a unique identifier, often" +
			" used to refetch an object or as key for a cache. The ID type" +
			" appears in a JSON response as a String; however, it is not" +
			" intended to be human-readable. When expected as an input" +
			" type, any string (such as '4') or integer (such as '4')" +
			" input value will be accepted as an ID.",
	}
)

type Int32Type int32

func (v Int32Type) MarshalBinary() ([]byte, error) {
	var bs [4]byte
	binary.LittleEndian.PutUint32(bs[:], uint32(v))
	return bs[:], nil
}

func (v Int32Type) MarshalText() ([]byte, error) {
	s := strconv.FormatInt(int64(v), 10)
	return []byte(s), nil
}

func (v Int32Type) MarshalJSON() ([]byte, error) {
	return json.Marshal(int32(v))
}

func (v *Int32Type) UnmarshalBinary(data []byte) error {
	val := binary.LittleEndian.Uint32(data)
	*v = Int32Type(val)
	return nil
}

func (v *Int32Type) UnmarshalText(text []byte) error {
	val, err := strconv.ParseInt(string(text), 10, 32)
	if err != nil {
		return err
	}
	*v = Int32Type(val)
	return nil
}

func newInt32Val() TypeValue {
	var v Int32Type
	return &v
}

type FloatType float64

func (v FloatType) MarshalBinary() ([]byte, error) {
	var bs [8]byte
	u := math.Float64bits(float64(v))
	binary.LittleEndian.PutUint64(bs[:], u)
	return bs[:], nil
}

func (v FloatType) MarshalText() ([]byte, error) {
	s := strconv.FormatFloat(float64(v), 'E', -1, 64)
	return []byte(s), nil
}

func (v FloatType) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(v))
}

func (v *FloatType) UnmarshalBinary(data []byte) error {
	u := binary.LittleEndian.Uint64(data)
	val := math.Float64frombits(u)
	*v = FloatType(val)
	return nil
}

func (v *FloatType) UnmarshalText(text []byte) error {
	val, err := strconv.ParseFloat(string(text), 64)
	if err != nil {
		return err
	}
	*v = FloatType(val)
	return nil
}

func newFloatVal() TypeValue {
	var v FloatType
	return &v
}

type StringType string

func (v StringType) MarshalBinary() ([]byte, error) {
	return []byte(v), nil
}

func (v StringType) MarshalText() ([]byte, error) {
	return v.MarshalBinary()
}

func (v StringType) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(v))
}

func (v *StringType) UnmarshalBinary(data []byte) error {
	s := string(data)
	*v = StringType(s)
	return nil
}

func (v *StringType) UnmarshalText(text []byte) error {
	return v.UnmarshalBinary(text)
}

func newStringVal() TypeValue {
	var v StringType
	return &v
}

type BoolType bool

func (v BoolType) MarshalBinary() ([]byte, error) {
	var bs [1]byte
	if v {
		bs[0] = 0
	} else {
		bs[0] = 1
	}
	return bs[:], nil
}

func (v BoolType) MarshalText() ([]byte, error) {
	s := strconv.FormatBool(bool(v))
	return []byte(s), nil
}

func (v BoolType) MarshalJSON() ([]byte, error) {
	return json.Marshal(bool(v))
}

func (v *BoolType) UnmarshalBinary(data []byte) error {
	if data[0] == 0 {
		*v = false
	} else if data[0] == 1 {
		*v = true
	} else {
		return x.Errorf("Invalid value for bool %v", data[0])
	}
	return nil
}

func (v *BoolType) UnmarshalText(text []byte) error {
	val, err := strconv.ParseBool(string(text))
	if err != nil {
		return err
	}
	*v = BoolType(val)
	return nil
}

func newBoolVal() TypeValue {
	var v BoolType
	return &v
}
