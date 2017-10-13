/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package query

import (
	"log"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/gogo/protobuf/proto"
)

// Codec implements the custom codec interface.
type Codec struct{}

// Marshal release the protos.Node pointers after marshalling the response.
func (c *Codec) Marshal(v interface{}) ([]byte, error) {
	var b []byte
	var err error
	switch val := v.(type) {
	case *protos.AssignedIds:
		b, err = proto.Marshal(val)
		if err != nil {
			return []byte{}, err
		}
		return b, nil
	case *protos.Version:
		b, err = proto.Marshal(val)
		if err != nil {
			return []byte{}, err
		}
		return b, nil

	case *protos.Response:
		b, err = proto.Marshal(val)
		if err != nil {
			return []byte{}, err
		}
		return b, nil

	default:
		log.Fatalf("Invalid type of value: %+v", v)
	}
	return b, nil
}

// Unmarshal constructs protos.Request from the byte slice.
func (c *Codec) Unmarshal(data []byte, v interface{}) error {
	check, ok := v.(*protos.Check)
	if ok {
		return proto.Unmarshal(data, check)
	}

	num, ok := v.(*protos.Num)
	if ok {
		return proto.Unmarshal(data, num)
	}

	r, ok := v.(*protos.Request)
	if !ok {
		log.Fatalf("Invalid type of value: %+v", v)
	}
	return proto.Unmarshal(data, r)
}

func (c *Codec) String() string {
	return "query.Codec"
}
