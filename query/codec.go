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
	r, ok := v.(*graphp.Response)
	if !ok {
		log.Fatalf("Invalid type of value: %+v", v)
	}

	b, err := proto.Marshal(r)
	if err != nil {
		return []byte{}, err
	}

	for _, it := range r.N {
		select {
		// Passing onto to channel which would put it into the sync pool.
		case nodeCh <- it:
		default:
		}
	}

	return b, nil
}

// Unmarshal constructs graphp.Request from the byte slice.
func (c *Codec) Unmarshal(data []byte, v interface{}) error {
	n, ok := v.(*graphp.Request)
	if !ok {
		log.Fatalf("Invalid type of value: %+v", v)
	}

	return proto.Unmarshal(data, n)
}

func (c *Codec) String() string {
	return "query.Codec"
}
