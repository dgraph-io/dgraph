/*
 * Copyright 2015-2018 Dgraph Labs, Inc. and Contributors
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

package posting

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math"
	"sort"

	"github.com/dgryski/go-farm"
	"github.com/golang/glog"

	bpb "github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/y"
	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

var (
	// ErrRetry can be triggered if the posting list got deleted from memory due to a hard commit.
	// In such a case, retry.
	ErrRetry = fmt.Errorf("Temporary error. Please retry")
	// ErrNoValue would be returned if no value was found in the posting list.
	ErrNoValue       = fmt.Errorf("No value found")
	errStopIteration = errors.New("Stop iteration")
	emptyPosting     = &pb.Posting{}
	maxListSize      = mb / 2
)

const (
	// Set means overwrite in mutation layer. It contributes 0 in Length.
	Set uint32 = 0x01
	// Del means delete in mutation layer. It contributes -1 in Length.
	Del uint32 = 0x02

	// BitSchemaPosting signals that the value stores a schema or type.
	BitSchemaPosting byte = 0x01
	// BitDeltaPosting signals that the value stores the delta of a posting list.
	BitDeltaPosting byte = 0x04
	// BitCompletePosting signals that the values stores a complete posting list.
	BitCompletePosting byte = 0x08
	// BitEmptyPosting signals that the value stores an empty posting list.
	BitEmptyPosting byte = 0x10
)

// List stores the in-memory representation of a posting list.
type List struct {
	x.SafeMutex
	key         []byte
	plist       *pb.PostingList
	mutationMap map[uint64]*pb.PostingList
	minTs       uint64 // commit timestamp of immutable layer, reject reads before this ts.
	maxTs       uint64 // max commit timestamp seen for this list.
}

func (l *List) maxVersion() uint64 {
	l.RLock()
	defer l.RUnlock()
	return l.maxTs
}

type pIterator struct {
	l          *List
	plist      *pb.PostingList
	uidPosting *pb.Posting
	pidx       int // index of postings
	plen       int

	dec  *codec.Decoder
	uids []uint64
	uidx int // Offset into the uids slice

	afterUid uint64
	splitIdx int
	// The timestamp of a delete marker in the mutable layer. If this value is greater
	// than zero, then the immutable posting list should not be traversed.
	deleteBelowTs uint64
}

func (it *pIterator) init(l *List, afterUid, deleteBelowTs uint64) error {
	if deleteBelowTs > 0 && deleteBelowTs <= l.minTs {
		return fmt.Errorf("deleteBelowTs (%d) must be greater than the minTs in the list (%d)",
			deleteBelowTs, l.minTs)
	}

	it.l = l
	it.splitIdx = it.selectInitialSplit(afterUid)
	if len(it.l.plist.Splits) > 0 {
		plist, err := l.readListPart(it.l.plist.Splits[it.splitIdx])
		if err != nil {
			return err
		}
		it.plist = plist
	} else {
		it.plist = l.plist
	}

	it.afterUid = afterUid
	it.deleteBelowTs = deleteBelowTs

	it.uidPosting = &pb.Posting{}
	it.dec = &codec.Decoder{Pack: it.plist.Pack}
	it.uids = it.dec.Seek(it.afterUid, codec.SeekCurrent)
	it.uidx = 0

	it.plen = len(it.plist.Postings)
	it.pidx = sort.Search(it.plen, func(idx int) bool {
		p := it.plist.Postings[idx]
		return it.afterUid < p.Uid
	})
	return nil
}

func (it *pIterator) selectInitialSplit(afterUid uint64) int {
	if afterUid == 0 {
		return 0
	}

	for i, startUid := range it.l.plist.Splits {
		// If startUid == afterUid, the current block should be selected.
		if startUid == afterUid {
			return i
		}
		// If this split starts at an UID greater than afterUid, there might be
		// elements in the previous split that need to be checked.
		if startUid > afterUid {
			return i - 1
		}
	}

	// In case no split's startUid is greater or equal than afterUid, start the
	// iteration at the start of the last split.
	return len(it.l.plist.Splits) - 1
}

// moveToNextPart re-initializes the iterator at the start of the next list part.
func (it *pIterator) moveToNextPart() error {
	it.splitIdx++
	plist, err := it.l.readListPart(it.l.plist.Splits[it.splitIdx])
	if err != nil {
		return err
	}
	it.plist = plist

	it.dec = &codec.Decoder{Pack: it.plist.Pack}
	// codec.SeekCurrent makes sure we skip returning afterUid during seek.
	it.uids = it.dec.Seek(it.afterUid, codec.SeekCurrent)
	it.uidx = 0

	it.plen = len(it.plist.Postings)
	it.pidx = sort.Search(it.plen, func(idx int) bool {
		p := it.plist.Postings[idx]
		return it.afterUid < p.Uid
	})

	return nil
}

// moveToNextValidPart moves the iterator to the next part that contains valid data.
// This is used to skip over parts of the list that might not contain postings.
func (it *pIterator) moveToNextValidPart() error {
	// Not a multi-part list, the iterator has reached the end of the list.
	if len(it.l.plist.Splits) == 0 {
		return nil
	}

	// If there are no more UIDs to iterate over, move to the next part of the
	// list that contains valid data.
	if len(it.uids) == 0 {
		for it.splitIdx <= len(it.l.plist.Splits)-2 {
			// moveToNextPart will increment it.splitIdx. Therefore, the for loop must only
			// continue until len(splits) - 2.
			if err := it.moveToNextPart(); err != nil {
				return err
			}

			if len(it.uids) > 0 {
				return nil
			}
		}
	}
	return nil
}

func (it *pIterator) next() error {
	if it.deleteBelowTs > 0 {
		it.uids = nil
		return nil
	}

	it.uidx++
	if it.uidx < len(it.uids) {
		return nil
	}
	it.uidx = 0
	it.uids = it.dec.Next()

	return it.moveToNextValidPart()
}

func (it *pIterator) valid() (bool, error) {
	if len(it.uids) > 0 {
		return true, nil
	}

	if err := it.moveToNextValidPart(); err != nil {
		return false, err
	} else if len(it.uids) > 0 {
		return true, nil
	}
	return false, nil
}

func (it *pIterator) posting() *pb.Posting {
	uid := it.uids[it.uidx]

	for it.pidx < it.plen {
		p := it.plist.Postings[it.pidx]
		if p.Uid > uid {
			break
		}
		if p.Uid == uid {
			return p
		}
		it.pidx++
	}
	it.uidPosting.Uid = uid
	return it.uidPosting
}

// ListOptions is used in List.Uids (in posting) to customize our output list of
// UIDs, for each posting list. It should be pb.to this package.
type ListOptions struct {
	ReadTs    uint64
	AfterUid  uint64   // Any UIDs returned must be after this value.
	Intersect *pb.List // Intersect results with this list of UIDs.
}

// NewPosting takes the given edge and returns its equivalent representation as a posting.
func NewPosting(t *pb.DirectedEdge) *pb.Posting {
	var op uint32
	if t.Op == pb.DirectedEdge_SET {
		op = Set
	} else if t.Op == pb.DirectedEdge_DEL {
		op = Del
	} else {
		x.Fatalf("Unhandled operation: %+v", t)
	}

	var postingType pb.Posting_PostingType
	if len(t.Lang) > 0 {
		postingType = pb.Posting_VALUE_LANG
	} else if t.ValueId == 0 {
		postingType = pb.Posting_VALUE
	} else {
		postingType = pb.Posting_REF
	}

	return &pb.Posting{
		Uid:         t.ValueId,
		Value:       t.Value,
		ValType:     t.ValueType,
		PostingType: postingType,
		LangTag:     []byte(t.Lang),
		Label:       t.Label,
		Op:          op,
		Facets:      t.Facets,
	}
}

func hasDeleteAll(mpost *pb.Posting) bool {
	return mpost.Op == Del && bytes.Equal(mpost.Value, []byte(x.Star)) && len(mpost.LangTag) == 0
}

// Ensure that you either abort the uncommitted postings or commit them before calling me.
func (l *List) updateMutationLayer(mpost *pb.Posting) {
	l.AssertLock()
	x.AssertTrue(mpost.Op == Set || mpost.Op == Del)

	// If we have a delete all, then we replace the map entry with just one.
	if hasDeleteAll(mpost) {
		plist := &pb.PostingList{}
		plist.Postings = append(plist.Postings, mpost)
		l.mutationMap[mpost.StartTs] = plist
		return
	}

	plist, ok := l.mutationMap[mpost.StartTs]
	if !ok {
		plist := &pb.PostingList{}
		plist.Postings = append(plist.Postings, mpost)
		l.mutationMap[mpost.StartTs] = plist
		return
	}
	// Even if we have a delete all in this transaction, we should still pick up any updates since.
	for i, prev := range plist.Postings {
		if prev.Uid == mpost.Uid {
			plist.Postings[i] = mpost
			return
		}
	}
	plist.Postings = append(plist.Postings, mpost)
}

// TypeID returns the typeid of destination vertex
func TypeID(edge *pb.DirectedEdge) types.TypeID {
	if edge.ValueId != 0 {
		return types.UidID
	}
	return types.TypeID(edge.ValueType)
}

func fingerprintEdge(t *pb.DirectedEdge) uint64 {
	// There could be a collision if the user gives us a value with Lang = "en" and later gives
	// us a value = "en" for the same predicate. We would end up overwritting his older lang
	// value.

	// All edges with a value without LANGTAG, have the same UID. In other words,
	// an (entity, attribute) can only have one untagged value.
	var id uint64 = math.MaxUint64

	// Value with a lang type.
	if len(t.Lang) > 0 {
		id = farm.Fingerprint64([]byte(t.Lang))
	} else if schema.State().IsList(t.Attr) {
		// TODO - When values are deleted for list type, then we should only delete the UID from
		// index if no other values produces that index token.
		// Value for list type.
		id = farm.Fingerprint64(t.Value)
	}
	return id
}

// canMutateUid returns an error if all the following conditions are met.
// * Predicate is of type UidID.
// * Predicate is not set to a list of UIDs in the schema.
// * The existing posting list has an entry that does not match the proposed
//   mutation's UID.
// In this case, the user should delete the existing predicate and retry, or mutate
// the schema to allow for multiple UIDs. This method is necessary to support UID
// predicates with single values because previously all UID predicates were
// considered lists.
// This functions returns a nil error in all other cases.
func (l *List) canMutateUid(txn *Txn, edge *pb.DirectedEdge) error {
	l.AssertRLock()

	if types.TypeID(edge.ValueType) != types.UidID {
		return nil
	}

	if schema.State().IsList(edge.Attr) {
		return nil
	}

	return l.iterate(txn.StartTs, 0, func(obj *pb.Posting) error {
		if obj.Uid != edge.GetValueId() {
			return fmt.Errorf(
				"cannot add value with uid %x to predicate %s because one of the existing "+
					"values does not match this uid, either delete the existing values first or "+
					"modify the schema to '%s: [uid]'",
				edge.GetValueId(), edge.Attr, edge.Attr)
		}
		return nil
	})
}

func (l *List) addMutation(ctx context.Context, txn *Txn, t *pb.DirectedEdge) error {
	l.Lock()
	defer l.Unlock()
	return l.addMutationInternal(ctx, txn, t)
}

func (l *List) addMutationInternal(ctx context.Context, txn *Txn, t *pb.DirectedEdge) error {
	l.AssertLock()

	if txn.ShouldAbort() {
		return y.ErrConflict
	}

	getKey := func(key []byte, uid uint64) string {
		return fmt.Sprintf("%s|%d", key, uid)
	}

	// We ensure that commit marks are applied to posting lists in the right
	// order. We can do so by proposing them in the same order as received by the Oracle delta
	// stream from Zero, instead of in goroutines.
	var conflictKey string
	if schema.State().HasUpsert(t.Attr) {
		// Consider checking to see if a email id is unique. A user adds:
		// <uid> <email> "email@email.org", and there's a string equal tokenizer
		// and upsert directive on the schema.
		// Then keys are "<email> <uid>" and "<email> email@email.org"
		// The first key won't conflict, because two different UIDs can try to
		// get the same email id. But, the second key would. Thus, we ensure
		// that two users don't set the same email id.
		conflictKey = getKey(l.key, 0)

	} else if x.Parse(l.key).IsData() {
		// Unless upsert is specified, we don't check for index conflicts, only
		// data conflicts.
		// If the data is of type UID, then we use SPO for conflict detection.
		// Otherwise, we use SP (for string, date, int, etc.).
		typ, err := schema.State().TypeOf(t.Attr)
		if err != nil {
			glog.V(2).Infof("Unable to find type of attr: %s. Err: %v", t.Attr, err)
			// Don't check for conflict.
		} else if typ == types.UidID {
			conflictKey = getKey(l.key, t.ValueId)
		} else {
			conflictKey = getKey(l.key, 0)
		}
	}

	mpost := NewPosting(t)
	mpost.StartTs = txn.StartTs
	if mpost.PostingType != pb.Posting_REF {
		t.ValueId = fingerprintEdge(t)
		mpost.Uid = t.ValueId
	}

	l.updateMutationLayer(mpost)
	txn.addConflictKey(conflictKey)
	return nil
}

// getMutation returns a marshaled version of posting list mutation stored internally.
func (l *List) getMutation(startTs uint64) []byte {
	l.RLock()
	defer l.RUnlock()
	if pl, ok := l.mutationMap[startTs]; ok {
		data, err := pl.Marshal()
		x.Check(err)
		return data
	}
	return nil
}

func (l *List) setMutation(startTs uint64, data []byte) {
	pl := new(pb.PostingList)
	x.Check(pl.Unmarshal(data))

	l.Lock()
	l.mutationMap[startTs] = pl
	l.Unlock()
}

// Iterate will allow you to iterate over this posting List, while having acquired a read lock.
// So, please keep this iteration cheap, otherwise mutations would get stuck.
// The iteration will start after the provided UID. The results would not include this uid.
// The function will loop until either the posting List is fully iterated, or you return a false
// in the provided function, which will indicate to the function to break out of the iteration.
//
// 	pl.Iterate(..., func(p *pb.posting) error {
//    // Use posting p
//    return nil // to continue iteration.
//    return errStopIteration // to break iteration.
//  })
func (l *List) Iterate(readTs uint64, afterUid uint64, f func(obj *pb.Posting) error) error {
	l.RLock()
	defer l.RUnlock()
	return l.iterate(readTs, afterUid, f)
}

// pickPostings goes through the mutable layer and returns the appropriate postings,
// along with the timestamp of the delete marker, if any. If this timestamp is greater
// than zero, it indicates that the immutable layer should be ignored during traversals.
// If greater than zero, this timestamp must thus be greater than l.minTs.
func (l *List) pickPostings(readTs uint64) (uint64, []*pb.Posting) {
	// This function would return zero ts for entries above readTs.
	effective := func(start, commit uint64) uint64 {
		if commit > 0 && commit <= readTs {
			// Has been committed and below the readTs.
			return commit
		}
		if start == readTs {
			// This mutation is by ME. So, I must be able to read it.
			return start
		}
		return 0
	}

	// First pick up the postings.
	var deleteBelowTs uint64
	var posts []*pb.Posting
	for startTs, plist := range l.mutationMap {
		// Pick up the transactions which are either committed, or the one which is ME.
		effectiveTs := effective(startTs, plist.CommitTs)
		if effectiveTs > deleteBelowTs {
			// We're above the deleteBelowTs marker. We wouldn't reach here if effectiveTs is zero.
			for _, mpost := range plist.Postings {
				if hasDeleteAll(mpost) {
					deleteBelowTs = effectiveTs
					continue
				}
				posts = append(posts, mpost)
			}
		}
	}

	if deleteBelowTs > 0 {
		// There was a delete all marker. So, trim down the list of postings.
		result := posts[:0]
		for _, post := range posts {
			effectiveTs := effective(post.StartTs, post.CommitTs)
			if effectiveTs < deleteBelowTs { // Do pick the posts at effectiveTs == deleteBelowTs.
				continue
			}
			result = append(result, post)
		}
		posts = result
	}

	// Sort all the postings by UID (inc order), then by commit/startTs in dec order.
	sort.Slice(posts, func(i, j int) bool {
		pi := posts[i]
		pj := posts[j]
		if pi.Uid == pj.Uid {
			ei := effective(pi.StartTs, pi.CommitTs)
			ej := effective(pj.StartTs, pj.CommitTs)
			return ei > ej // Pick the higher, so we can discard older commits for the same UID.
		}
		return pi.Uid < pj.Uid
	})
	return deleteBelowTs, posts
}

func (l *List) iterate(readTs uint64, afterUid uint64, f func(obj *pb.Posting) error) error {
	l.AssertRLock()

	deleteBelowTs, mposts := l.pickPostings(readTs)
	if readTs < l.minTs {
		return errors.Errorf("readTs: %d less than minTs: %d for key: %q", readTs, l.minTs, l.key)
	}

	midx, mlen := 0, len(mposts)
	if afterUid > 0 {
		midx = sort.Search(mlen, func(idx int) bool {
			mp := mposts[idx]
			return afterUid < mp.Uid
		})
	}

	var (
		mp, pp  *pb.Posting
		pitr    pIterator
		prevUid uint64
		err     error
	)
	err = pitr.init(l, afterUid, deleteBelowTs)
	if err != nil {
		return err
	}
	for err == nil {
		if midx < mlen {
			mp = mposts[midx]
		} else {
			mp = emptyPosting
		}
		if valid, err := pitr.valid(); err != nil {
			return err
		} else if valid {
			pp = pitr.posting()
		} else {
			pp = emptyPosting
		}

		switch {
		case mp.Uid > 0 && mp.Uid == prevUid:
			// Only pick the latest version of this posting.
			// mp.Uid can be zero if it's an empty posting.
			midx++
		case pp.Uid == 0 && mp.Uid == 0:
			// Reached empty posting for both iterators.
			return nil
		case mp.Uid == 0 || (pp.Uid > 0 && pp.Uid < mp.Uid):
			// Either mp is empty, or pp is lower than mp.
			err = f(pp)
			if err := pitr.next(); err != nil {
				return err
			}
		case pp.Uid == 0 || (mp.Uid > 0 && mp.Uid < pp.Uid):
			// Either pp is empty, or mp is lower than pp.
			if mp.Op != Del {
				err = f(mp)
			}
			prevUid = mp.Uid
			midx++
		case pp.Uid == mp.Uid:
			if mp.Op != Del {
				err = f(mp)
			}
			prevUid = mp.Uid
			if err := pitr.next(); err != nil {
				return err
			}
			midx++
		default:
			log.Fatalf("Unhandled case during iteration of posting list.")
		}
	}
	if err == errStopIteration {
		return nil
	}
	return err
}

func (l *List) IsEmpty(readTs, afterUid uint64) (bool, error) {
	l.RLock()
	defer l.RUnlock()
	var count int
	err := l.iterate(readTs, afterUid, func(p *pb.Posting) error {
		count++
		return errStopIteration
	})
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

func (l *List) length(readTs, afterUid uint64) int {
	l.AssertRLock()
	count := 0
	err := l.iterate(readTs, afterUid, func(p *pb.Posting) error {
		count++
		return nil
	})
	if err != nil {
		return -1
	}
	return count
}

// Length iterates over the mutation layer and counts number of elements.
func (l *List) Length(readTs, afterUid uint64) int {
	l.RLock()
	defer l.RUnlock()
	return l.length(readTs, afterUid)
}

// Rollup performs the rollup process, merging the immutable and mutable layers
// and outputting the resulting list so it can be written to disk.
// During this process, the list might be split into multiple lists if the main
// list or any of the existing parts become too big.
//
// A normal list has the following format:
// <key> -> <posting list with all the data for this list>
//
// A multi-part list is stored in multiple keys. The keys for the parts will be generated by
// appending the first UID in the part to the key. The list will have the following format:
// <key> -> <posting list that includes no postings but a list of each part's start UID>
// <key, 1> -> <first part of the list with all the data for this part>
// <key, next start UID> -> <second part of the list with all the data for this part>
// ...
// <key, last start UID> -> <last part of the list with all its data>
//
// The first part of a multi-part list always has start UID 1 and will be the last part
// to be deleted, at which point the entire list will be marked for deletion.
// As the list grows, existing parts might be split if they become too big.
func (l *List) Rollup() ([]*bpb.KV, error) {
	l.RLock()
	defer l.RUnlock()
	out, err := l.rollup(math.MaxUint64)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, nil
	}

	var kvs []*bpb.KV
	kv := &bpb.KV{}
	kv.Version = out.newMinTs
	kv.Key = l.key
	val, meta := marshalPostingList(out.plist)
	kv.UserMeta = []byte{meta}
	kv.Value = val
	kvs = append(kvs, kv)

	for startUid, plist := range out.parts {
		// Any empty posting list would still have BitEmpty set. And the main posting list
		// would NOT have that posting list startUid in the splits list.
		kv := out.marshalPostingListPart(l.key, startUid, plist)
		kvs = append(kvs, kv)
	}

	return kvs, nil
}

func (out *rollupOutput) marshalPostingListPart(
	baseKey []byte, startUid uint64, plist *pb.PostingList) *bpb.KV {
	kv := &bpb.KV{}
	kv.Version = out.newMinTs
	kv.Key = x.GetSplitKey(baseKey, startUid)
	val, meta := marshalPostingList(plist)
	kv.UserMeta = []byte{meta}
	kv.Value = val

	return kv
}

func marshalPostingList(plist *pb.PostingList) ([]byte, byte) {
	if isPlistEmpty(plist) {
		return nil, BitEmptyPosting
	}

	data, err := plist.Marshal()
	x.Check(err)
	return data, BitCompletePosting
}

const blockSize int = 256

type rollupOutput struct {
	plist    *pb.PostingList
	parts    map[uint64]*pb.PostingList
	newMinTs uint64
}

// Merge all entries in mutation layer with commitTs <= l.commitTs into
// immutable layer. Note that readTs can be math.MaxUint64, so do NOT use it
// directly. It should only serve as the read timestamp for iteration.
func (l *List) rollup(readTs uint64) (*rollupOutput, error) {
	l.AssertRLock()

	// Pick all committed entries
	if l.minTs > readTs {
		// If we are already past the readTs, then skip the rollup.
		return nil, nil
	}

	out := &rollupOutput{
		plist: &pb.PostingList{
			Splits: l.plist.Splits,
		},
		parts: make(map[uint64]*pb.PostingList),
	}

	var plist *pb.PostingList
	var enc codec.Encoder
	var startUid, endUid uint64
	var splitIdx int

	// Method to properly initialize all the variables described above.
	init := func() {
		enc = codec.Encoder{BlockSize: blockSize}

		// If not a multi-part list, all UIDs go to the same encoder.
		if len(l.plist.Splits) == 0 {
			plist = out.plist
			endUid = math.MaxUint64
			return
		}

		// Otherwise, load the corresponding part and set endUid to correctly
		// detect the end of the list.
		startUid = l.plist.Splits[splitIdx]
		if splitIdx+1 == len(l.plist.Splits) {
			endUid = math.MaxUint64
		} else {
			endUid = l.plist.Splits[splitIdx+1] - 1
		}

		plist = &pb.PostingList{}
	}

	init()
	err := l.iterate(readTs, 0, func(p *pb.Posting) error {
		if p.Uid > endUid {
			plist.Pack = enc.Done()
			out.parts[startUid] = plist

			splitIdx++
			init()
		}

		enc.Add(p.Uid)
		if p.Facets != nil || p.PostingType != pb.Posting_REF || len(p.Label) != 0 {
			plist.Postings = append(plist.Postings, p)
		}
		return nil
	})
	// Finish  writing the last part of the list (or the whole list if not a multi-part list).
	x.Check(err)
	plist.Pack = enc.Done()
	if len(l.plist.Splits) > 0 {
		out.parts[startUid] = plist
	}

	maxCommitTs := l.minTs
	{
		// We can't rely upon iterate to give us the max commit timestamp, because it can skip over
		// postings which had deletions to provide a sorted view of the list. Therefore, the safest
		// way to get the max commit timestamp is to pick all the relevant postings for the given
		// readTs and calculate the maxCommitTs.
		// If deleteBelowTs is greater than zero, there was a delete all marker. The list of
		// postings has been trimmed down.
		deleteBelowTs, mposts := l.pickPostings(readTs)
		maxCommitTs = x.Max(maxCommitTs, deleteBelowTs)
		for _, mp := range mposts {
			maxCommitTs = x.Max(maxCommitTs, mp.CommitTs)
		}
	}

	// Check if the list (or any of it's parts if it's been previously split) have
	// become too big. Split the list if that is the case.
	out.newMinTs = maxCommitTs
	out.splitUpList()
	out.removeEmptySplits()
	return out, nil
}

// ApproxLen returns an approximate count of the UIDs in the posting list.
func (l *List) ApproxLen() int {
	l.RLock()
	defer l.RUnlock()
	return len(l.mutationMap) + codec.ApproxLen(l.plist.Pack)
}

// Uids returns the UIDs given some query params.
// We have to apply the filtering before applying (offset, count).
// WARNING: Calling this function just to get UIDs is expensive
func (l *List) Uids(opt ListOptions) (*pb.List, error) {
	// Pre-assign length to make it faster.
	l.RLock()
	// Use approximate length for initial capacity.
	res := make([]uint64, 0, len(l.mutationMap)+codec.ApproxLen(l.plist.Pack))
	out := &pb.List{}
	if len(l.mutationMap) == 0 && opt.Intersect != nil && len(l.plist.Splits) == 0 {
		if opt.ReadTs < l.minTs {
			l.RUnlock()
			return out, ErrTsTooOld
		}
		algo.IntersectCompressedWith(l.plist.Pack, opt.AfterUid, opt.Intersect, out)
		l.RUnlock()
		return out, nil
	}

	err := l.iterate(opt.ReadTs, opt.AfterUid, func(p *pb.Posting) error {
		if p.PostingType == pb.Posting_REF {
			res = append(res, p.Uid)
		}
		return nil
	})
	l.RUnlock()
	if err != nil {
		return out, err
	}

	// Do The intersection here as it's optimized.
	out.Uids = res
	if opt.Intersect != nil {
		algo.IntersectWith(out, opt.Intersect, out)
	}
	return out, nil
}

// Postings calls postFn with the postings that are common with
// UIDs in the opt ListOptions.
func (l *List) Postings(opt ListOptions, postFn func(*pb.Posting) error) error {
	l.RLock()
	defer l.RUnlock()

	return l.iterate(opt.ReadTs, opt.AfterUid, func(p *pb.Posting) error {
		if p.PostingType != pb.Posting_REF {
			return nil
		}
		return postFn(p)
	})
}

// AllUntaggedValues returns all the values in the posting list with no language tag.
func (l *List) AllUntaggedValues(readTs uint64) ([]types.Val, error) {
	l.RLock()
	defer l.RUnlock()

	var vals []types.Val
	err := l.iterate(readTs, 0, func(p *pb.Posting) error {
		if len(p.LangTag) == 0 {
			vals = append(vals, types.Val{
				Tid:   types.TypeID(p.ValType),
				Value: p.Value,
			})
		}
		return nil
	})
	return vals, err
}

// AllValues returns all the values in the posting list.
func (l *List) AllValues(readTs uint64) ([]types.Val, error) {
	l.RLock()
	defer l.RUnlock()

	var vals []types.Val
	err := l.iterate(readTs, 0, func(p *pb.Posting) error {
		vals = append(vals, types.Val{
			Tid:   types.TypeID(p.ValType),
			Value: p.Value,
		})
		return nil
	})
	return vals, err
}

// GetLangTags finds the language tags of each posting in the list.
func (l *List) GetLangTags(readTs uint64) ([]string, error) {
	l.RLock()
	defer l.RUnlock()

	var tags []string
	err := l.iterate(readTs, 0, func(p *pb.Posting) error {
		tags = append(tags, string(p.LangTag))
		return nil
	})
	return tags, err
}

// Value returns the default value from the posting list. The default value is
// defined as the value without a language tag.
func (l *List) Value(readTs uint64) (rval types.Val, rerr error) {
	l.RLock()
	defer l.RUnlock()
	val, found, err := l.findValue(readTs, math.MaxUint64)
	if err != nil {
		return val, err
	}
	if !found {
		return val, ErrNoValue
	}
	return val, nil
}

// ValueFor returns a value from posting list, according to preferred language list.
// If list is empty, value without language is returned; if such value is not
// available, value with smallest UID is returned.
// If list consists of one or more languages, first available value is returned.
// If no language from the list matches the values, processing is the same as for empty list.
func (l *List) ValueFor(readTs uint64, langs []string) (rval types.Val, rerr error) {
	l.RLock() // All public methods should acquire locks, while private ones should assert them.
	defer l.RUnlock()
	p, err := l.postingFor(readTs, langs)
	if err != nil {
		return rval, err
	}
	return valueToTypesVal(p), nil
}

func (l *List) postingFor(readTs uint64, langs []string) (p *pb.Posting, rerr error) {
	l.AssertRLock() // Avoid recursive locking by asserting a lock here.
	return l.postingForLangs(readTs, langs)
}

// ValueForTag returns the value in the posting list with the given language tag.
func (l *List) ValueForTag(readTs uint64, tag string) (rval types.Val, rerr error) {
	l.RLock()
	defer l.RUnlock()
	p, err := l.postingForTag(readTs, tag)
	if err != nil {
		return rval, err
	}
	return valueToTypesVal(p), nil
}

func valueToTypesVal(p *pb.Posting) (rval types.Val) {
	// This is ok because we dont modify the value of a posting. We create a newPosting
	// and add it to the PostingList to do a set.
	rval.Value = p.Value
	rval.Tid = types.TypeID(p.ValType)
	return
}

func (l *List) postingForLangs(readTs uint64, langs []string) (pos *pb.Posting, rerr error) {
	l.AssertRLock()

	any := false
	// look for language in preferred order
	for _, lang := range langs {
		if lang == "." {
			any = true
			break
		}
		pos, rerr = l.postingForTag(readTs, lang)
		if rerr == nil {
			return pos, nil
		}
	}

	// look for value without language
	if any || len(langs) == 0 {
		if found, pos, err := l.findPosting(readTs, math.MaxUint64); err != nil {
			return nil, err
		} else if found {
			return pos, nil
		}
	}

	var found bool
	// last resort - return value with smallest lang UID.
	if any {
		err := l.iterate(readTs, 0, func(p *pb.Posting) error {
			if p.PostingType == pb.Posting_VALUE_LANG {
				pos = p
				found = true
				return errStopIteration
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	if found {
		return pos, nil
	}

	return pos, ErrNoValue
}

func (l *List) postingForTag(readTs uint64, tag string) (p *pb.Posting, rerr error) {
	l.AssertRLock()
	uid := farm.Fingerprint64([]byte(tag))
	found, p, err := l.findPosting(readTs, uid)
	if err != nil {
		return p, err
	}
	if !found {
		return p, ErrNoValue
	}

	return p, nil
}

func (l *List) findValue(readTs, uid uint64) (rval types.Val, found bool, err error) {
	l.AssertRLock()
	found, p, err := l.findPosting(readTs, uid)
	if !found {
		return rval, found, err
	}

	return valueToTypesVal(p), true, nil
}

func (l *List) findPosting(readTs uint64, uid uint64) (found bool, pos *pb.Posting, err error) {
	// Iterate starts iterating after the given argument, so we pass UID - 1
	err = l.iterate(readTs, uid-1, func(p *pb.Posting) error {
		if p.Uid == uid {
			pos = p
			found = true
		}
		return errStopIteration
	})

	return found, pos, err
}

// Facets gives facets for the posting representing value.
func (l *List) Facets(readTs uint64, param *pb.FacetParams, langs []string) (fs []*api.Facet,
	ferr error) {
	l.RLock()
	defer l.RUnlock()
	p, err := l.postingFor(readTs, langs)
	if err != nil {
		return nil, err
	}
	return facets.CopyFacets(p.Facets, param), nil
}

func (l *List) readListPart(startUid uint64) (*pb.PostingList, error) {
	key := x.GetSplitKey(l.key, startUid)
	txn := pstore.NewTransactionAt(l.minTs, false)
	item, err := txn.Get(key)
	if err != nil {
		return nil, err
	}
	part := &pb.PostingList{}
	if err := unmarshalOrCopy(part, item); err != nil {
		return nil, err
	}
	return part, nil
}

// shouldSplit returns true if the given plist should be split in two.
func shouldSplit(plist *pb.PostingList) bool {
	return plist.Size() >= maxListSize && len(plist.Pack.Blocks) > 1
}

// splitUpList checks the list and splits it in smaller parts if needed.
func (out *rollupOutput) splitUpList() {
	// Contains the posting lists that should be split.
	var lists []*pb.PostingList

	// If list is not split yet, insert the main list.
	if len(out.plist.Splits) == 0 {
		lists = append(lists, out.plist)
	}

	// Insert the split lists if they exist.
	for _, startUid := range out.splits() {
		part := out.parts[startUid]
		lists = append(lists, part)
	}

	// List of startUids for each list part after the splitting process is complete.
	var newSplits []uint64

	for i, list := range lists {
		startUid := uint64(1)
		// If the list is split, select the right startUid for this list.
		if len(out.plist.Splits) > 0 {
			startUid = out.plist.Splits[i]
		}

		if shouldSplit(list) {
			// Split the list. Update out.splits with the new lists and add their
			// start UIDs to the list of new splits.
			startUids, pls := binSplit(startUid, list)
			for i, startUid := range startUids {
				out.parts[startUid] = pls[i]
				newSplits = append(newSplits, startUid)
			}
		} else {
			// No need to split the list. Add the startUid to the array of new splits.
			newSplits = append(newSplits, startUid)
		}
	}

	// No new lists were created so there's no need to update the list of splits.
	if len(newSplits) == len(lists) {
		return
	}

	// The splits changed so update them.
	out.plist = &pb.PostingList{
		Splits: newSplits,
	}
}

// binSplit takes the given plist and returns two new plists, each with
// half of the blocks and postings of the original as well as the new startUids
// for each of the new parts.
func binSplit(lowUid uint64, plist *pb.PostingList) ([]uint64, []*pb.PostingList) {
	midBlock := len(plist.Pack.Blocks) / 2
	midUid := plist.Pack.Blocks[midBlock].GetBase()

	// Generate posting list holding the first half of the current list's postings.
	lowPl := new(pb.PostingList)
	lowPl.Pack = &pb.UidPack{
		BlockSize: plist.Pack.BlockSize,
		Blocks:    plist.Pack.Blocks[:midBlock],
	}

	// Generate posting list holding the second half of the current list's postings.
	highPl := new(pb.PostingList)
	highPl.Pack = &pb.UidPack{
		BlockSize: plist.Pack.BlockSize,
		Blocks:    plist.Pack.Blocks[midBlock:],
	}

	// Add elements in plist.Postings to the corresponding list.
	for _, posting := range plist.Postings {
		if posting.Uid < midUid {
			lowPl.Postings = append(lowPl.Postings, posting)
		} else {
			highPl.Postings = append(highPl.Postings, posting)
		}
	}

	return []uint64{lowUid, midUid}, []*pb.PostingList{lowPl, highPl}
}

// removeEmptySplits updates the split list by removing empty posting lists' startUids.
func (out *rollupOutput) removeEmptySplits() {
	var splits []uint64
	for startUid, plist := range out.parts {
		// Do not remove the first split for now, as every multi-part list should always
		// have a split starting with UID 1.
		if startUid == 1 {
			splits = append(splits, startUid)
			continue
		}

		if !isPlistEmpty(plist) {
			splits = append(splits, startUid)
		}
	}
	out.plist.Splits = splits
	sortSplits(splits)

	if len(out.plist.Splits) == 1 {
		// Only the first split remains. If it's also empty, remove it as well.
		// This should mark the entire list for deletion.
		if isPlistEmpty(out.parts[1]) {
			out.plist.Splits = []uint64{}
		}
	}
}

// Returns the sorted list of start UIDs based on the keys in out.parts.
// out.parts is considered the source of truth so this method is considered
// safer than using out.plist.Splits directly.
func (out *rollupOutput) splits() []uint64 {
	var splits []uint64
	for startUid := range out.parts {
		splits = append(splits, startUid)
	}
	sortSplits(splits)
	return splits
}

// isPlistEmpty returns true if the given plist is empty. Plists with splits are
// considered non-empty.
func isPlistEmpty(plist *pb.PostingList) bool {
	if len(plist.Splits) > 0 {
		return false
	}
	if plist.Pack == nil || len(plist.Pack.Blocks) == 0 {
		return true
	}
	return false
}

func sortSplits(splits []uint64) {
	sort.Slice(splits, func(i, j int) bool {
		return splits[i] < splits[j]
	})
}

// PartSplits returns an empty array if the list has not been split into multiple parts.
// Otherwise, it returns an array containing the start UID of each part.
func (l *List) PartSplits() []uint64 {
	splits := make([]uint64, len(l.plist.Splits))
	copy(splits, l.plist.Splits)
	return splits
}
