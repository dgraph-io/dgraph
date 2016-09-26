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
	"time"

	"github.com/dgraph-io/dgraph/x"
)

func (to Scalar) Convert(value TypeValue) (TypeValue, error) {
	if to.Id() == stringId {
		// If we are converting to a string, simply use
		// MarshalText
		if r, err := value.MarshalText(); err != nil {
			return nil, err
		} else {
			return StringType(r), nil
		}
	}

	u := to.Unmarshaler
	// Otherwise we check if the conversion is defined.
	switch v := value.(type) {
	case StringType:
		// If the value is a string, then we can always Unmarshal it using
		// the unmarshaller
		return u.FromText([]byte(v))
	case Int32Type:
		if c, ok := u.(int32Unmarshaler); !ok {
			return nil, cantConvert(to, v)
		} else {
			return c.fromInt(int32(v))
		}

	case FloatType:
		if c, ok := u.(floatUnmarshaler); !ok {
			return nil, cantConvert(to, v)
		} else {
			return c.fromFloat(float64(v))
		}

	case BoolType:
		if c, ok := u.(boolUnmarshaler); !ok {
			return nil, cantConvert(to, v)
		} else {
			return c.fromBool(bool(v))
		}

	case time.Time:
		if c, ok := u.(timeUnmarshaler); !ok {
			return nil, cantConvert(to, v)
		} else {
			return c.fromTime(v)
		}

	default:
		return nil, cantConvert(to, v)
	}
}

func cantConvert(to Scalar, val TypeValue) error {
	return x.Errorf("Cannot convert %v to type %s", val, to.Name)
}

type int32Unmarshaler interface {
	fromInt(value int32) (TypeValue, error)
}

type floatUnmarshaler interface {
	fromFloat(value float64) (TypeValue, error)
}

type boolUnmarshaler interface {
	fromBool(value bool) (TypeValue, error)
}

type timeUnmarshaler interface {
	fromTime(value time.Time) (TypeValue, error)
}
