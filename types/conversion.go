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

import (
	"time"

	"github.com/dgraph-io/dgraph/x"
)

// Convert converts the value to given scalar type.
func (to Scalar) Convert(value Value) (Value, error) {
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
		c, ok := u.(int32Unmarshaler)
		if !ok {
			return nil, cantConvert(to, v)
		}
		if err := c.fromInt(int32(*v)); err != nil {
			return nil, err
		}

	case *Float:
		c, ok := u.(floatUnmarshaler)
		if !ok {
			return nil, cantConvert(to, v)
		}
		if err := c.fromFloat(float64(*v)); err != nil {
			return nil, err
		}

	case *Bool:
		c, ok := u.(boolUnmarshaler)
		if !ok {
			return nil, cantConvert(to, v)
		}
		if err := c.fromBool(bool(*v)); err != nil {
			return nil, err
		}

	case *Time:
		c, ok := u.(timeUnmarshaler)
		if !ok {
			return nil, cantConvert(to, v)
		}
		if err := c.fromTime(v.Time); err != nil {
			return nil, err
		}

	case *Date:
		c, ok := u.(dateUnmarshaler)
		if !ok {
			return nil, cantConvert(to, v)
		}
		if err := c.fromDate(*v); err != nil {
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

type int32Unmarshaler interface {
	fromInt(value int32) error
}

type floatUnmarshaler interface {
	fromFloat(value float64) error
}

type boolUnmarshaler interface {
	fromBool(value bool) error
}

type timeUnmarshaler interface {
	fromTime(value time.Time) error
}

type dateUnmarshaler interface {
	fromDate(value Date) error
}
