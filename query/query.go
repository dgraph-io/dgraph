/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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

package query

import (
	"fmt"
	"math"

	"github.com/google/flatbuffers/go"
	"github.com/manishrjain/dgraph/posting"
	"github.com/manishrjain/dgraph/posting/types"
	"github.com/manishrjain/dgraph/query/result"
	"github.com/manishrjain/dgraph/uid"
	"github.com/manishrjain/dgraph/x"
)

// Aim to get this query working:
// {
//   me {
//     id
//     firstName
//     lastName
//     birthday {
//       month
//       day
//     }
//     friends {
//       name
//     }
//   }
// }

var log = x.Log("query")

type SubGraph struct {
	Attr     string
	Children []*SubGraph

	Query  []byte
	Result []byte
}

func NewGraph(id uint64, xid string) *SubGraph {
	// This would set the Result field in SubGraph,
	// and populate the children for attributes.
	return nil
}

type Mattr struct {
	Attr   string
	Msg    *Mattr
	Query  []byte // flatbuffer
	Result []byte // flatbuffer

	/*
		ResultUids  []byte // Flatbuffer result.Uids
		ResultValue []byte // gob.Encode
	*/
}

type Message struct {
	Id    uint64 // Dgraph Id
	Xid   string // External Id
	Attrs []Mattr
}

type Node struct {
	Id  uint64
	Xid string
}

func extract(l *posting.List, uids *[]byte, value *[]byte) error {
	b := flatbuffers.NewBuilder(0)
	var p types.Posting

	llen := l.Length()
	if ok := l.Get(&p, l.Length()-1); ok {
		if p.Uid() == math.MaxUint64 {
			// Contains a value posting, not useful for Uids vector.
			llen -= 1
		}
	}

	result.UidsStartUidVector(b, llen)
	for i := l.Length() - 1; i >= 0; i-- {
		if ok := l.Get(&p, i); !ok {
			return fmt.Errorf("While retrieving posting")
		}
		if p.Uid() == math.MaxUint64 {
			*value = make([]byte, p.ValueLength())
			copy(*value, p.ValueBytes())

		} else {
			b.PrependUint64(p.Uid())
		}
	}
	vend := b.EndVector(llen)

	result.UidsStart(b)
	result.UidsAddUid(b, vend)
	end := result.UidsEnd(b)
	b.Finish(end)

	buf := b.Bytes[b.Head():]
	*uids = make([]byte, len(buf))
	copy(*uids, buf)
	return nil
}

func Run(m *Message) error {
	if len(m.Xid) > 0 {
		u, err := uid.GetOrAssign(m.Xid)
		if err != nil {
			x.Err(log, err).WithField("xid", m.Xid).Error(
				"While GetOrAssign uid from external id")
			return err
		}
		log.WithField("xid", m.Xid).WithField("uid", u).Debug("GetOrAssign")
		m.Id = u
	}

	if m.Id == 0 {
		err := fmt.Errorf("Query internal id is zero")
		x.Err(log, err).Error("Invalid query")
		return err
	}

	for idx := range m.Attrs {
		mattr := &m.Attrs[idx]
		key := posting.Key(m.Id, mattr.Attr)
		pl := posting.Get(key)

		if err := extract(pl, &mattr.ResultUids, &mattr.ResultValue); err != nil {
			x.Err(log, err).WithField("uid", m.Id).WithField("attr", mattr.Attr).
				Error("While extracting data from posting list")
		}

		if mattr.Msg != nil {
			// Now this would most likely be sent over wire to other servers.
			if err := Run(mattr.Msg); err != nil {
				return err
			}
		}
	}
	return nil
}
