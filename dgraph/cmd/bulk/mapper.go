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
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

type mapper struct {
	*state
	shards []shardState // shard is based on predicate
	mePool *sync.Pool
}

type shardState struct {
	// Buffer up map entries until we have a sufficient amount, then sort and
	// write them to file.
	entries     []*pb.MapEntry
	encodedSize uint64
	mu          sync.Mutex // Allow only 1 write per shard at a time.
}

func newMapper(st *state) *mapper {
	return &mapper{
		state:  st,
		shards: make([]shardState, st.opt.MapShards),
		mePool: &sync.Pool{
			New: func() interface{} {
				return &pb.MapEntry{}
			},
		},
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

func (m *mapper) openOutputFile(shardIdx int) (*os.File, error) {
	fileNum := atomic.AddUint32(&m.mapFileId, 1)
	filename := filepath.Join(
		m.opt.TmpDir,
		"shards",
		fmt.Sprintf("%03d", shardIdx),
		fmt.Sprintf("%06d.map.gz", fileNum),
	)
	x.Check(os.MkdirAll(filepath.Dir(filename), 0755))
	return os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
}

func (m *mapper) writeMapEntriesToFile(entries []*pb.MapEntry, encodedSize uint64, shardIdx int) {
	defer m.shards[shardIdx].mu.Unlock() // Locked by caller.

	sort.Slice(entries, func(i, j int) bool {
		return less(entries[i], entries[j])
	})

	f, err := m.openOutputFile(shardIdx)
	x.Check(err)

	defer func() {
		x.Check(f.Sync())
		x.Check(f.Close())
	}()

	gzWriter := gzip.NewWriter(f)
	w := bufio.NewWriter(gzWriter)
	defer func() {
		x.Check(w.Flush())
		x.Check(gzWriter.Flush())
		x.Check(gzWriter.Close())
	}()

	sizeBuf := make([]byte, binary.MaxVarintLen64)
	for _, me := range entries {
		n := binary.PutUvarint(sizeBuf, uint64(me.Size()))
		_, err := w.Write(sizeBuf[:n])
		x.Check(err)

		meBuf, err := me.Marshal()
		x.Check(err)
		_, err = w.Write(meBuf)
		x.Check(err)
		m.mePool.Put(me)
	}
}

func (m *mapper) run(inputFormat chunker.InputFormat) {
	chunker := chunker.NewChunker(inputFormat, 1000)
	nquads := chunker.NQuads()
	go func() {
		for chunkBuf := range m.readerChunkCh {
			if err := chunker.Parse(chunkBuf); err != nil {
				atomic.AddInt64(&m.prog.errCount, 1)
				if !m.opt.IgnoreErrors {
					x.Check(err)
				}
			}
		}
		nquads.Flush()
	}()

	for nqs := range nquads.Ch() {
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
			if sh.encodedSize >= m.opt.MapBufSize {
				sh.mu.Lock() // One write at a time.
				go m.writeMapEntriesToFile(sh.entries, sh.encodedSize, i)
				// Clear the entries and encodedSize for the next batch.
				// Proactively allocate 32 slots to bootstrap the entries slice.
				sh.entries = make([]*pb.MapEntry, 0, 32)
				sh.encodedSize = 0
			}
		}
	}

	for i := range m.shards {
		sh := &m.shards[i]
		if len(sh.entries) > 0 {
			sh.mu.Lock() // One write at a time.
			m.writeMapEntriesToFile(sh.entries, sh.encodedSize, i)
		}
		m.shards[i].mu.Lock() // Ensure that the last file write finishes.
	}
}

func (m *mapper) addMapEntry(key []byte, p *pb.Posting, shard int) {
	atomic.AddInt64(&m.prog.mapEdgeCount, 1)

	me := m.mePool.Get().(*pb.MapEntry)
	*me = pb.MapEntry{Key: key}

	if p.PostingType != pb.Posting_REF || len(p.Facets) > 0 {
		me.Posting = p
	} else {
		me.Uid = p.Uid
	}
	sh := &m.shards[shard]

	var err error
	sh.entries = append(sh.entries, me)
	sh.encodedSize += uint64(me.Size())
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
