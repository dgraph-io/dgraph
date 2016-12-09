/*
 * Copyright 2016 Dgraph Labs, Inc.
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

import "github.com/dgraph-io/dgraph/x"

// Convert converts the value to given scalar type.
func (to Scalar) Convert(value Value) (Value, error) {
	if to.ID() == value.Type().ID() {
		return value, nil
	}

	if to.ID() == StringID || to.ID() == BytesID {
		// If we are converting to a string or bytes, simply use MarshalText
		r, err := value.MarshalText()
		if err != nil {
			return nil, err
		}
		v := String(r)
		return &v, nil
	}

	u := ValueForType(to.ID())
	c := u.(Unmarshaler)
	// Otherwise we check if the conversion is defined.
	switch v := value.(type) {
	case *Bytes:
		// Bytes convert the same way as strings, as bytes denote an untyped value which is almost
		// always a string.
		if err := u.UnmarshalText([]byte(*v)); err != nil {
			return nil, err
		}

	case *String:
		// If the value is a string, then we can always Unmarshal it using the unmarshaller
		if err := u.UnmarshalText([]byte(*v)); err != nil {
			return nil, err
		}

	case *Int32:
		if err := c.fromInt(*v); err != nil {
			return nil, err
		}

	case *Float:
		if err := c.fromFloat(*v); err != nil {
			return nil, err
		}

	case *Bool:
		if err := c.fromBool(*v); err != nil {
			return nil, err
		}
	case *Time:
		if err := c.fromTime(*v); err != nil {
			return nil, err
		}

	case *Date:
		if err := c.fromDate(*v); err != nil {
			return nil, err
		}
	case *Geo:
		if err := c.fromGeo(*v); err != nil {
			return nil, err
		}
	default:
		return nil, cantConvert(to, v)
	}
	return u, nil
}

func cantConvert(to Scalar, val Value) error {
	return x.Errorf("Cannot convert %v to type %s", val, to.Name)
}

type Unmarshaler interface {
	fromInt(value Int32) error
	fromFloat(value Float) error
	fromBool(value Bool) error
	fromTime(value Time) error
	fromDate(value Date) error
	fromGeo(value Geo) error
}
