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

	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/gogo/protobuf/proto"
)

// Codec implements the custom codec interface.
type Codec struct{}

// Marshal release the graphp.Node pointers after marshalling the response.
func (c *Codec) Marshal(v interface{}) ([]byte, error) {
	var b []byte
	var err error
	switch val := v.(type) {
	case *graphp.Version:
		b, err = proto.Marshal(val)
		if err != nil {
			return []byte{}, err
		}
		return b, nil

	case *graphp.Response:
		b, err = proto.Marshal(val)
		if err != nil {
			return []byte{}, err
		}

		for _, it := range val.N {
			select {
			// Passing onto to channel which would put it into the sync pool.
			case nodeCh <- it:
			default:
			}
		}
	default:
		log.Fatalf("Invalid type of value: %+v", v)
	}
	return b, nil
}

// Unmarshal constructs graphp.Request from the byte slice.
func (c *Codec) Unmarshal(data []byte, v interface{}) error {
	check, ok := v.(*graphp.Check)
	if ok {
		return proto.Unmarshal(data, check)
	}

	r, ok := v.(*graphp.Request)
	if !ok {
		log.Fatalf("Invalid type of value: %+v", v)
	}
	return proto.Unmarshal(data, r)
}

func (c *Codec) String() string {
	return "query.Codec"
}
