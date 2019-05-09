/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package bulk

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
	"github.com/gogo/protobuf/proto"
)

type mapper struct {
	*state
	shards []shardState // shard is based on predicate
}

type shardState struct {
	// Buffer up map entries until we have a sufficient amount, then sort and
	// write them to file.
	entriesBuf []byte
	mu         sync.Mutex // Allow only 1 write per shard at a time.
}

func newMapper(st *state) *mapper {
	return &mapper{
		state:  st,
		shards: make([]shardState, st.opt.MapShards),
	}
}

func less(lhs, rhs *pb.MapEntry) bool {
	if keyCmp := bytes.Compare(lhs.Key, rhs.Key); keyCmp != 0 {
		return keyCmp < 0
	}
	lhsUID := lhs.Uid
	rhsUID := rhs.Uid
	if lhs.Posting != nil {
		lhsUID = lhs.Posting.Uid
	}
	if rhs.Posting != nil {
		rhsUID = rhs.Posting.Uid
	}
	return lhsUID < rhsUID
}

func (m *mapper) writeMapEntriesToFile(entriesBuf []byte, shardIdx int) {
	defer m.shards[shardIdx].mu.Unlock() // Locked by caller.

	buf := entriesBuf
	var entries []*pb.MapEntry
	for len(buf) > 0 {
		sz, n := binary.Uvarint(buf)
		x.AssertTrue(n > 0)
		buf = buf[n:]
		me := new(pb.MapEntry)
		x.Check(proto.Unmarshal(buf[:sz], me))
		buf = buf[sz:]
		entries = append(entries, me)
	}

	sort.Slice(entries, func(i, j int) bool {
		return less(entries[i], entries[j])
	})

	buf = entriesBuf
	for _, me := range entries {
		n := binary.PutUvarint(buf, uint64(me.Size()))
		buf = buf[n:]
		n, err := me.MarshalTo(buf)
		x.Check(err)
		buf = buf[n:]
	}
	x.AssertTrue(len(buf) == 0)

	fileNum := atomic.AddUint32(&m.mapFileId, 1)
	filename := filepath.Join(
		m.opt.TmpDir,
		"shards",
		fmt.Sprintf("%03d", shardIdx),
		fmt.Sprintf("%06d.map", fileNum),
	)
	x.Check(os.MkdirAll(filepath.Dir(filename), 0755))
	x.Check(x.WriteFileSync(filename, entriesBuf, 0644))
}

func (m *mapper) run(inputFormat int) {
	chunker := chunker.NewChunker(inputFormat)
	for chunkBuf := range m.readerChunkCh {
		done := false
		for !done {
			nqs, err := chunker.Parse(chunkBuf)
			if err == io.EOF {
				done = true
			} else if err != nil {
				atomic.AddInt64(&m.prog.errCount, 1)
				if !m.opt.IgnoreErrors {
					x.Check(err)
				}
			}

			for _, nq := range nqs {
				if err := facets.SortAndValidate(nq.Facets); err != nil {
					atomic.AddInt64(&m.prog.errCount, 1)
					if !m.opt.IgnoreErrors {
						x.Check(err)
					}
				}

				m.processNQuad(gql.NQuad{NQuad: nq})
				atomic.AddInt64(&m.prog.nquadCount, 1)
			}

			for i := range m.shards {
				sh := &m.shards[i]
				if len(sh.entriesBuf) >= int(m.opt.MapBufSize) {
					sh.mu.Lock() // One write at a time.
					go m.writeMapEntriesToFile(sh.entriesBuf, i)
					sh.entriesBuf = make([]byte, 0, m.opt.MapBufSize*11/10)
				}
			}
		}
	}

	for i := range m.shards {
		sh := &m.shards[i]
		if len(sh.entriesBuf) > 0 {
			sh.mu.Lock() // One write at a time.
			m.writeMapEntriesToFile(sh.entriesBuf, i)
		}
		m.shards[i].mu.Lock() // Ensure that the last file write finishes.
	}
}

func (m *mapper) addMapEntry(key []byte, p *pb.Posting, shard int) {
	atomic.AddInt64(&m.prog.mapEdgeCount, 1)

	me := &pb.MapEntry{
		Key: key,
	}
	if p.PostingType != pb.Posting_REF || len(p.Facets) > 0 {
		me.Posting = p
	} else {
		me.Uid = p.Uid
	}
	sh := &m.shards[shard]

	var err error
	sh.entriesBuf = x.AppendUvarint(sh.entriesBuf, uint64(me.Size()))
	sh.entriesBuf, err = x.AppendProtoMsg(sh.entriesBuf, me)
	x.Check(err)
}

func (m *mapper) processNQuad(nq gql.NQuad) {
	sid := m.uid(nq.GetSubject())
	var oid uint64
	var de *pb.DirectedEdge
	if nq.GetObjectValue() == nil {
		oid = m.uid(nq.GetObjectId())
		de = nq.CreateUidEdge(sid, oid)
	} else {
		var err error
		de, err = nq.CreateValueEdge(sid)
		x.Check(err)
	}

	fwd, rev := m.createPostings(nq, de)
	shard := m.state.shards.shardFor(nq.Predicate)
	key := x.DataKey(nq.Predicate, sid)
	m.addMapEntry(key, fwd, shard)

	if rev != nil {
		key = x.ReverseKey(nq.Predicate, oid)
		m.addMapEntry(key, rev, shard)
	}
	m.addIndexMapEntries(nq, de)
}

func (m *mapper) uid(xid string) uint64 {
	if !m.opt.NewUids {
		if uid, err := strconv.ParseUint(xid, 0, 64); err == nil {
			m.xids.BumpTo(uid)
			return uid
		}
	}

	return m.lookupUid(xid)
}

func (m *mapper) lookupUid(xid string) uint64 {
	uid := m.xids.AssignUid(xid)
	if !m.opt.StoreXids {
		return uid
	}
	if strings.HasPrefix(xid, "_:") {
		// Don't store xids for blank nodes.
		return uid
	}
	nq := gql.NQuad{NQuad: &api.NQuad{
		Subject:   xid,
		Predicate: "xid",
		ObjectValue: &api.Value{
			Val: &api.Value_StrVal{StrVal: xid},
		},
	}}
	m.processNQuad(nq)
	return uid
}

func (m *mapper) createPostings(nq gql.NQuad,
	de *pb.DirectedEdge) (*pb.Posting, *pb.Posting) {

	m.schema.validateType(de, nq.ObjectValue == nil)

	p := posting.NewPosting(de)
	sch := m.schema.getSchema(nq.GetPredicate())
	if nq.GetObjectValue() != nil {
		if lang := de.GetLang(); len(lang) > 0 {
			p.Uid = farm.Fingerprint64([]byte(lang))
		} else if sch.List {
			p.Uid = farm.Fingerprint64(de.Value)
		} else {
			p.Uid = math.MaxUint64
		}
	}
	p.Facets = nq.Facets

	// Early exit for no reverse edge.
	if sch.GetDirective() != pb.SchemaUpdate_REVERSE {
		return p, nil
	}

	// Reverse predicate
	x.AssertTruef(nq.GetObjectValue() == nil, "only has reverse schema if object is UID")
	de.Entity, de.ValueId = de.ValueId, de.Entity
	m.schema.validateType(de, true)
	rp := posting.NewPosting(de)

	de.Entity, de.ValueId = de.ValueId, de.Entity // de reused so swap back.

	return p, rp
}

func (m *mapper) addIndexMapEntries(nq gql.NQuad, de *pb.DirectedEdge) {
	if nq.GetObjectValue() == nil {
		return // Cannot index UIDs
	}

	sch := m.schema.getSchema(nq.GetPredicate())
	for _, tokerName := range sch.GetTokenizer() {
		// Find tokeniser.
		toker, ok := tok.GetTokenizer(tokerName)
		if !ok {
			log.Fatalf("unknown tokenizer %q", tokerName)
		}

		// Create storage value.
		storageVal := types.Val{
			Tid:   types.TypeID(de.GetValueType()),
			Value: de.GetValue(),
		}

		// Convert from storage type to schema type.
		schemaVal, err := types.Convert(storageVal, types.TypeID(sch.GetValueType()))
		// Shouldn't error, since we've already checked for convertibility when
		// doing edge postings. So okay to be fatal.
		x.Check(err)

		// Extract tokens.
		toks, err := tok.BuildTokens(schemaVal.Value, tok.GetLangTokenizer(toker, nq.Lang))
		x.Check(err)

		// Store index posting.
		for _, t := range toks {
			m.addMapEntry(
				x.IndexKey(nq.Predicate, t),
				&pb.Posting{
					Uid:         de.GetEntity(),
					PostingType: pb.Posting_REF,
				},
				m.state.shards.shardFor(nq.Predicate),
			)
		}
	}
}
