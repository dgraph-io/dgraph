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
	"github.com/dgraph-io/ristretto/z"
	farm "github.com/dgryski/go-farm"
)

type mapper struct {
	*state
	shards []shardState // shard is based on predicate
}

type shardState struct {
	// Buffer up map entries until we have a sufficient amount, then sort and
	// write them to file.
	cbuf *z.Buffer
	mu   sync.Mutex // Allow only 1 write per shard at a time.
}

func newMapper(st *state) *mapper {
	shards := make([]shardState, st.opt.MapShards)
	for i := range shards {
		shards[i].cbuf = z.NewBuffer(1 << 20)
	}
	return &mapper{
		state:  st,
		shards: shards,
	}
}

type MapEntry []byte

// type mapEntry struct {
// 	uid   uint64 // if plist is filled, then corresponds to plist's uid.
// 	key   []byte
// 	plist []byte
// }

func mapEntrySize(key []byte, p *pb.Posting) int {
	return 8 + 4 + 4 + len(key) + p.Size()
}

func marshalMapEntry(dst []byte, uid uint64, key []byte, p *pb.Posting) {
	if p != nil {
		uid = p.Uid
	}
	binary.BigEndian.PutUint64(dst[0:8], uid)
	binary.BigEndian.PutUint32(dst[8:12], uint32(len(key)))

	psz := p.Size()
	binary.BigEndian.PutUint32(dst[12:16], uint32(psz))

	n := copy(dst[16:], key)

	if psz > 0 {
		pbuf := dst[16+n:]
		_, err := p.MarshalToSizedBuffer(pbuf[:psz])
		x.Check(err)
	}

	x.AssertTrue(len(dst) == 16+n+psz)
}

func (me MapEntry) Size() int {
	return len(me)
}

func (me MapEntry) Uid() uint64 {
	return binary.BigEndian.Uint64(me[0:8])
}

func (me MapEntry) Key() []byte {
	sz := binary.BigEndian.Uint32(me[8:12])
	return me[16 : 16+sz]
}

func (me MapEntry) Plist() []byte {
	ksz := binary.BigEndian.Uint32(me[8:12])
	sz := binary.BigEndian.Uint32(me[12:16])
	start := 16 + ksz
	return me[start : start+sz]
}

func less(lhs, rhs MapEntry) bool {
	if keyCmp := bytes.Compare(lhs.Key(), rhs.Key()); keyCmp != 0 {
		return keyCmp < 0
	}
	return lhs.Uid() < rhs.Uid()
}

func (m *mapper) openOutputFile(shardIdx int) (*os.File, error) {
	fileNum := atomic.AddUint32(&m.mapFileId, 1)
	filename := filepath.Join(
		m.opt.TmpDir,
		mapShardDir,
		fmt.Sprintf("%03d", shardIdx),
		fmt.Sprintf("%06d.map.gz", fileNum),
	)
	x.Check(os.MkdirAll(filepath.Dir(filename), 0750))
	return os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
}

func (m *mapper) writeMapEntriesToFile(cbuf *z.Buffer, shardIdx int) {
	defer func() {
		m.shards[shardIdx].mu.Unlock() // Locked by caller.
		cbuf.Release()
	}()

	offsets := cbuf.SliceOffsets(nil)

	sort.Slice(offsets, func(i, j int) bool {
		lhs := MapEntry(cbuf.Slice(offsets[i]))
		rhs := MapEntry(cbuf.Slice(offsets[j]))
		return less(lhs, rhs)
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

	// Create partition keys for the map file.
	header := &pb.MapHeader{
		PartitionKeys: [][]byte{},
	}

	var bufSize int64
	for _, off := range offsets {
		me := MapEntry(cbuf.Slice(off))
		bufSize += int64(4 + len(me))
		if bufSize < m.opt.PartitionBufSize {
			continue
		}
		sz := len(header.PartitionKeys)
		if sz > 0 && bytes.Equal(me.Key(), header.PartitionKeys[sz-1]) {
			// We already have this key.
			continue
		}
		header.PartitionKeys = append(header.PartitionKeys, me.Key())
		bufSize = 0
	}
	// Write the header to the map file.
	headerBuf, err := header.Marshal()
	x.Check(err)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(headerBuf)))
	x.Check2(w.Write(lenBuf))
	x.Check2(w.Write(headerBuf))
	x.Check(err)

	sizeBuf := make([]byte, binary.MaxVarintLen64)
	for _, off := range offsets {
		me := cbuf.Slice(off)
		n := binary.PutUvarint(sizeBuf, uint64(len(me)))
		_, err := w.Write(sizeBuf[:n])
		x.Check(err)

		_, err = w.Write(me)
		x.Check(err)
	}
}

func (m *mapper) run(inputFormat chunker.InputFormat) {
	chunk := chunker.NewChunker(inputFormat, 1000)
	nquads := chunk.NQuads()
	go func() {
		for chunkBuf := range m.readerChunkCh {
			if err := chunk.Parse(chunkBuf); err != nil {
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
			if uint64(sh.cbuf.Len()) >= m.opt.MapBufSize {
				sh.mu.Lock() // One write at a time.
				go m.writeMapEntriesToFile(sh.cbuf, i)
				// Clear the entries and encodedSize for the next batch.
				// Proactively allocate 32 slots to bootstrap the entries slice.
				sh.cbuf = z.NewBuffer(1 << 20)
			}
		}
	}

	for i := range m.shards {
		sh := &m.shards[i]
		if sh.cbuf.Len() > 0 {
			sh.mu.Lock() // One write at a time.
			m.writeMapEntriesToFile(sh.cbuf, i)
		} else {
			sh.cbuf.Release()
		}
		m.shards[i].mu.Lock() // Ensure that the last file write finishes.
	}
}

func (m *mapper) addMapEntry(key []byte, p *pb.Posting, shard int) {
	atomic.AddInt64(&m.prog.mapEdgeCount, 1)

	uid := p.Uid
	if p.PostingType != pb.Posting_REF || len(p.Facets) > 0 {
		// Keep p
	} else {
		// We only needed the UID.
		p = nil
	}

	sh := &m.shards[shard]

	sz := mapEntrySize(key, p)
	dst := sh.cbuf.SliceAllocate(sz)
	marshalMapEntry(dst, uid, key, p)
}

func (m *mapper) processNQuad(nq gql.NQuad) {
	sid := m.uid(nq.GetSubject())
	if sid == 0 {
		panic(fmt.Sprintf("invalid UID with value 0 for %v", nq.GetSubject()))
	}
	var oid uint64
	var de *pb.DirectedEdge
	if nq.GetObjectValue() == nil {
		oid = m.uid(nq.GetObjectId())
		if oid == 0 {
			panic(fmt.Sprintf("invalid UID with value 0 for %v", nq.GetObjectId()))
		}
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
	// We create a copy of xid string here because it is stored in
	// the map in AssignUid and going to be around throughout the process.
	// We don't want to keep the whole line that we read from file alive.
	// xid is a substring of the line that we read from the file and if
	// xid is alive, the whole line is going to be alive and won't be GC'd.
	// Also, checked that sb goes on the stack whereas sb.String() goes on
	// heap. Note that the calls to the strings.Builder.* are inlined.
	sb := strings.Builder{}
	x.Check2(sb.WriteString(xid))
	uid, isNew := m.xids.AssignUid(sb.String())
	if !m.opt.StoreXids || !isNew {
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
		lang := de.GetLang()
		switch {
		case len(lang) > 0:
			p.Uid = farm.Fingerprint64([]byte(lang))
		case sch.List:
			p.Uid = farm.Fingerprint64(de.Value)
		default:
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
		toks, err := tok.BuildTokens(schemaVal.Value, tok.GetTokenizerForLang(toker, nq.Lang))
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
