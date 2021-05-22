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
	"encoding/hex"
	"log"
	"math"
	"sort"

	"github.com/dgryski/go-farm"
	"github.com/pkg/errors"

	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/badger/v3/y"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/dgraph-io/sroar"
	"github.com/golang/protobuf/proto"
)

var (
	// ErrRetry can be triggered if the posting list got deleted from memory due to a hard commit.
	// In such a case, retry.
	ErrRetry = errors.New("Temporary error. Please retry")
	// ErrNoValue would be returned if no value was found in the posting list.
	ErrNoValue = errors.New("No value found")
	// ErrStopIteration is returned when an iteration is terminated early.
	ErrStopIteration = errors.New("Stop iteration")
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

// NewList returns a new list with an immutable layer set to plist and the
// timestamp of the immutable layer set to minTs.
func NewList(key []byte, plist *pb.PostingList, minTs uint64) *List {
	return &List{
		key:         key,
		plist:       plist,
		mutationMap: make(map[uint64]*pb.PostingList),
		minTs:       minTs,
	}
}

func (l *List) maxVersion() uint64 {
	l.RLock()
	defer l.RUnlock()
	return l.maxTs
}

// pIterator only iterates over Postings. Not UIDs.
type pIterator struct {
	l     *List
	plist *pb.PostingList
	pidx  int // index of postings

	afterUid uint64
	splitIdx int
	// The timestamp of a delete marker in the mutable layer. If this value is greater
	// than zero, then the immutable posting list should not be traversed.
	deleteBelowTs uint64
}

func (it *pIterator) seek(l *List, afterUid, deleteBelowTs uint64) error {
	if deleteBelowTs > 0 && deleteBelowTs <= l.minTs {
		return errors.Errorf("deleteBelowTs (%d) must be greater than the minTs in the list (%d)",
			deleteBelowTs, l.minTs)
	}

	it.l = l
	it.splitIdx = it.selectInitialSplit(afterUid)
	if len(it.l.plist.Splits) > 0 {
		plist, err := l.readListPart(it.l.plist.Splits[it.splitIdx])
		if err != nil {
			return errors.Wrapf(err, "cannot read initial list part for list with base key %s",
				hex.EncodeToString(l.key))
		}
		it.plist = plist
	} else {
		it.plist = l.plist
	}

	it.afterUid = afterUid
	it.deleteBelowTs = deleteBelowTs
	if deleteBelowTs > 0 {
		// We don't need to iterate over the immutable layer if this is > 0. Returning here would
		// mean it.uids is empty and valid() would return false.
		return nil
	}

	it.pidx = sort.Search(len(it.plist.Postings), func(idx int) bool {
		p := it.plist.Postings[idx]
		return it.afterUid < p.Uid
	})
	return nil
}

func (it *pIterator) selectInitialSplit(afterUid uint64) int {
	return it.l.splitIdx(afterUid)
}

// moveToNextValidPart moves the iterator to the next part that contains valid data.
// This is used to skip over parts of the list that might not contain postings.
func (it *pIterator) moveToNextValidPart() error {
	// Not a multi-part list, the iterator has reached the end of the list.
	splits := it.l.plist.Splits
	it.splitIdx++

	for ; it.splitIdx < len(splits); it.splitIdx++ {
		plist, err := it.l.readListPart(splits[it.splitIdx])
		if err != nil {
			return errors.Wrapf(err,
				"cannot move to next list part in iterator for list with key %s",
				hex.EncodeToString(it.l.key))
		}
		it.plist = plist
		if len(plist.Postings) == 0 {
			continue
		}
		if plist.Postings[0].Uid > it.afterUid {
			it.pidx = 0
			return nil
		}
		it.pidx = sort.Search(len(plist.Postings), func(idx int) bool {
			p := plist.Postings[idx]
			return it.afterUid < p.Uid
		})
		if it.pidx == len(plist.Postings) {
			continue
		}
		return nil
	}
	return nil
}

// valid asserts that pIterator has valid uids, or advances it to the next valid part.
// It returns false if there are no more valid parts.
func (it *pIterator) valid() (bool, error) {
	if it.deleteBelowTs > 0 {
		return false, nil
	}
	if it.pidx < len(it.plist.Postings) {
		return true, nil
	}

	err := it.moveToNextValidPart()
	switch {
	case err != nil:
		return false, errors.Wrapf(err, "cannot advance iterator when calling pIterator.valid")
	case it.pidx < len(it.plist.Postings):
		return true, nil
	default:
		return false, nil
	}
}

func (it *pIterator) posting() *pb.Posting {
	p := it.plist.Postings[it.pidx]
	return p
}

// ListOptions is used in List.Uids (in posting) to customize our output list of
// UIDs, for each posting list. It should be pb.to this package.
type ListOptions struct {
	ReadTs    uint64
	AfterUid  uint64   // Any UIDs returned must be after this value.
	Intersect *pb.List // Intersect results with this list of UIDs.
	First     int
}

// NewPosting takes the given edge and returns its equivalent representation as a posting.
func NewPosting(t *pb.DirectedEdge) *pb.Posting {
	var op uint32
	switch t.Op {
	case pb.DirectedEdge_SET:
		op = Set
	case pb.DirectedEdge_DEL:
		op = Del
	default:
		x.Fatalf("Unhandled operation: %+v", t)
	}

	var postingType pb.Posting_PostingType
	switch {
	case len(t.Lang) > 0:
		postingType = pb.Posting_VALUE_LANG
	case t.ValueId == 0:
		postingType = pb.Posting_VALUE
	default:
		postingType = pb.Posting_REF
	}

	p := &pb.Posting{
		Uid:         t.ValueId,
		Value:       t.Value,
		ValType:     t.ValueType,
		PostingType: postingType,
		LangTag:     []byte(t.Lang),
		Op:          op,
		Facets:      t.Facets,
	}
	return p
}

func hasDeleteAll(mpost *pb.Posting) bool {
	return mpost.Op == Del && bytes.Equal(mpost.Value, []byte(x.Star)) && len(mpost.LangTag) == 0
}

// Ensure that you either abort the uncommitted postings or commit them before calling me.
func (l *List) updateMutationLayer(mpost *pb.Posting, singleUidUpdate bool) error {
	l.AssertLock()
	x.AssertTrue(mpost.Op == Set || mpost.Op == Del)

	// If we have a delete all, then we replace the map entry with just one.
	if hasDeleteAll(mpost) {
		plist := &pb.PostingList{}
		plist.Postings = append(plist.Postings, mpost)
		if l.mutationMap == nil {
			l.mutationMap = make(map[uint64]*pb.PostingList)
		}
		l.mutationMap[mpost.StartTs] = plist
		return nil
	}

	plist, ok := l.mutationMap[mpost.StartTs]
	if !ok {
		plist = &pb.PostingList{}
		if l.mutationMap == nil {
			l.mutationMap = make(map[uint64]*pb.PostingList)
		}
		l.mutationMap[mpost.StartTs] = plist
	}

	if singleUidUpdate {
		// This handles the special case when adding a value to predicates of type uid.
		// The current value should be deleted in favor of this value. This needs to
		// be done because the fingerprint for the value is not math.MaxUint64 as is
		// the case with the rest of the scalar predicates.
		newPlist := &pb.PostingList{}
		newPlist.Postings = append(newPlist.Postings, mpost)

		// Add the deletions in the existing plist because those postings are not picked
		// up by iterating. Not doing so would result in delete operations that are not
		// applied when the transaction is committed.
		for _, post := range plist.Postings {
			if post.Op == Del && post.Uid != mpost.Uid {
				newPlist.Postings = append(newPlist.Postings, post)
			}
		}

		err := l.iterateAll(mpost.StartTs, 0, func(obj *pb.Posting) error {
			// Ignore values which have the same uid as they will get replaced
			// by the current value.
			if obj.Uid == mpost.Uid {
				return nil
			}

			// Mark all other values as deleted. By the end of the iteration, the
			// list of postings will contain deleted operations and only one set
			// for the mutation stored in mpost.
			objCopy := proto.Clone(obj).(*pb.Posting)
			objCopy.Op = Del
			newPlist.Postings = append(newPlist.Postings, objCopy)
			return nil
		})
		if err != nil {
			return err
		}

		// Update the mutation map with the new plist. Return here since the code below
		// does not apply for predicates of type uid.
		l.mutationMap[mpost.StartTs] = newPlist
		return nil
	}

	// Even if we have a delete all in this transaction, we should still pick up any updates since.
	// Note: If we have a big transaction of say 1M postings, then this loop would be taking up all
	// the time, because it is O(N^2), where N = number of postings added.
	for i, prev := range plist.Postings {
		if prev.Uid == mpost.Uid {
			plist.Postings[i] = mpost
			return nil
		}
	}
	plist.Postings = append(plist.Postings, mpost)
	return nil
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
	// us a value = "en" for the same predicate. We would end up overwriting his older lang
	// value.

	// All edges with a value without LANGTAG, have the same UID. In other words,
	// an (entity, attribute) can only have one untagged value.
	var id uint64 = math.MaxUint64

	// Value with a lang type.
	switch {
	case len(t.Lang) > 0:
		id = farm.Fingerprint64([]byte(t.Lang))
	case schema.State().IsList(t.Attr):
		// TODO - When values are deleted for list type, then we should only delete the UID from
		// index if no other values produces that index token.
		// Value for list type.
		id = farm.Fingerprint64(t.Value)
	}
	return id
}

func (l *List) addMutation(ctx context.Context, txn *Txn, t *pb.DirectedEdge) error {
	l.Lock()
	defer l.Unlock()
	return l.addMutationInternal(ctx, txn, t)
}

func GetConflictKey(pk x.ParsedKey, key []byte, t *pb.DirectedEdge) uint64 {
	getKey := func(key []byte, uid uint64) uint64 {
		// Instead of creating a string first and then doing a fingerprint, let's do a fingerprint
		// here to save memory allocations.
		// Not entirely sure about effect on collision chances due to this simple XOR with uid.
		return farm.Fingerprint64(key) ^ uid
	}

	var conflictKey uint64
	switch {
	case schema.State().HasNoConflict(t.Attr):
		break
	case schema.State().HasUpsert(t.Attr):
		// Consider checking to see if a email id is unique. A user adds:
		// <uid> <email> "email@email.org", and there's a string equal tokenizer
		// and upsert directive on the schema.
		// Then keys are "<email> <uid>" and "<email> email@email.org"
		// The first key won't conflict, because two different UIDs can try to
		// get the same email id. But, the second key would. Thus, we ensure
		// that two users don't set the same email id.
		conflictKey = getKey(key, 0)

	case pk.IsData() && schema.State().IsList(t.Attr):
		// Data keys, irrespective of whether they are UID or values, should be judged based on
		// whether they are lists or not. For UID, t.ValueId = UID. For value, t.ValueId =
		// fingerprint(value) or could be fingerprint(lang) or something else.
		//
		// For singular uid predicate, like partner: uid // no list.
		// a -> b
		// a -> c
		// Run concurrently, only one of them should succeed.
		// But for friend: [uid], both should succeed.
		//
		// Similarly, name: string
		// a -> "x"
		// a -> "y"
		// This should definitely have a conflict.
		// But, if name: [string], then they can both succeed.
		conflictKey = getKey(key, t.ValueId)

	case pk.IsData(): // NOT a list. This case must happen after the above case.
		conflictKey = getKey(key, 0)

	case pk.IsIndex() || pk.IsCountOrCountRev():
		// Index keys are by default of type [uid].
		conflictKey = getKey(key, t.ValueId)

	default:
		// Don't assign a conflictKey.
	}

	return conflictKey
}

func (l *List) addMutationInternal(ctx context.Context, txn *Txn, t *pb.DirectedEdge) error {
	l.AssertLock()

	if txn.ShouldAbort() {
		return x.ErrConflict
	}

	mpost := NewPosting(t)
	mpost.StartTs = txn.StartTs
	if mpost.PostingType != pb.Posting_REF {
		t.ValueId = fingerprintEdge(t)
		mpost.Uid = t.ValueId
	}

	// Check whether this mutation is an update for a predicate of type uid.
	pk, err := x.Parse(l.key)
	if err != nil {
		return errors.Wrapf(err, "cannot parse key when adding mutation to list with key %s",
			hex.EncodeToString(l.key))
	}
	pred, ok := schema.State().Get(ctx, t.Attr)
	isSingleUidUpdate := ok && !pred.GetList() && pred.GetValueType() == pb.Posting_UID &&
		pk.IsData() && mpost.Op == Set && mpost.PostingType == pb.Posting_REF

	if err != l.updateMutationLayer(mpost, isSingleUidUpdate) {
		return errors.Wrapf(err, "cannot update mutation layer of key %s with value %+v",
			hex.EncodeToString(l.key), mpost)
	}

	// We ensure that commit marks are applied to posting lists in the right
	// order. We can do so by proposing them in the same order as received by the Oracle delta
	// stream from Zero, instead of in goroutines.
	txn.addConflictKey(GetConflictKey(pk, l.key, t))
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
	if l.mutationMap == nil {
		l.mutationMap = make(map[uint64]*pb.PostingList)
	}
	l.mutationMap[startTs] = pl
	l.Unlock()
}

func (l *List) splitIdx(afterUid uint64) int {
	if afterUid == 0 || len(l.plist.Splits) == 0 {
		return 0
	}
	for i, startUid := range l.plist.Splits {
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
	return len(l.plist.Splits) - 1
}

func (l *List) Bitmap(opt ListOptions) (*sroar.Bitmap, error) {
	l.RLock()
	defer l.RUnlock()
	return l.bitmap(opt)
}

// Bitmap would generate a sroar.Bitmap from the list.
// It works on split posting lists as well.
func (l *List) bitmap(opt ListOptions) (*sroar.Bitmap, error) {
	deleteBelow, posts := l.pickPostings(opt.ReadTs)

	var iw *sroar.Bitmap
	if opt.Intersect != nil {
		iw = codec.FromList(opt.Intersect)
	}
	r := sroar.NewBitmap()
	if deleteBelow == 0 {
		r = sroar.FromBuffer(l.plist.Bitmap)
		if iw != nil {
			r.And(iw)
		}
		codec.RemoveRange(r, 0, opt.AfterUid)

		si := l.splitIdx(opt.AfterUid)
		for _, startUid := range l.plist.Splits[si:] {
			// We could skip over some splits, if they won't have the Uid range we care about.
			split, err := l.readListPart(startUid)
			if err != nil {
				return nil, errors.Wrapf(err, "while reading a split with startUid: %d", startUid)
			}
			s := sroar.FromBuffer(split.Bitmap)

			// Intersect with opt.Intersect.
			if iw != nil {
				s.And(iw)
			}
			if startUid < opt.AfterUid {
				// Only keep the Uids after opt.AfterUid.
				codec.RemoveRange(s, 0, opt.AfterUid)
			}
			r.Or(s)
		}
	}

	prev := uint64(0)
	for _, p := range posts {
		if p.Uid == prev {
			continue
		}
		if p.Op == Set {
			r.Set(p.Uid)
		} else if p.Op == Del {
			r.Remove(p.Uid)
		}
		prev = p.Uid
	}

	codec.RemoveRange(r, 0, opt.AfterUid)
	if iw != nil {
		r.And(iw)
	}
	return r, nil
}

// Iterate will allow you to iterate over the mutable and immutable layers of
// this posting List, while having acquired a read lock.
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

// IterateAll iterates over all the UIDs and Postings.
// TODO: We should remove this function after merging roaring bitmaps and fixing up how we map
// facetsMatrix to uidMatrix.
func (l *List) iterateAll(readTs uint64, afterUid uint64, f func(obj *pb.Posting) error) error {

	bm, err := l.bitmap(ListOptions{
		ReadTs:   readTs,
		AfterUid: afterUid,
	})
	if err != nil {
		return err
	}

	p := &pb.Posting{}

	uitr := bm.NewIterator()
	var next uint64

	advance := func() {
		next = math.MaxUint64
		if uitr.HasNext() {
			next = uitr.Next()
		}
	}
	advance()

	var maxUid uint64
	fn := func(obj *pb.Posting) error {
		maxUid = x.Max(maxUid, obj.Uid)
		return f(obj)
	}

	fi := func(obj *pb.Posting) error {
		for next < obj.Uid {
			p.Uid = next
			if err := fn(p); err != nil {
				return err
			}
			advance()
		}
		if err := fn(obj); err != nil {
			return err
		}
		if obj.Uid == next {
			advance()
		}
		return nil
	}
	if err := l.iterate(readTs, afterUid, fi); err != nil {
		return err
	}

	codec.RemoveRange(bm, 0, maxUid)
	uitr = bm.NewIterator()
	for uitr.HasNext() {
		p.Uid = uitr.Next()
		f(p)
	}
	return nil
}

func (l *List) IterateAll(readTs uint64, afterUid uint64, f func(obj *pb.Posting) error) error {
	l.RLock()
	defer l.RUnlock()
	return l.iterateAll(readTs, afterUid, f)
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

	// mposts is the list of mutable postings
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

	// pitr iterates through immutable postings
	err = pitr.seek(l, afterUid, deleteBelowTs)
	if err != nil {
		return errors.Wrapf(err, "cannot initialize iterator when calling List.iterate")
	}

loop:
	for err == nil {
		if midx < mlen {
			mp = mposts[midx]
		} else {
			mp = emptyPosting
		}

		valid, err := pitr.valid()
		switch {
		case err != nil:
			break loop
		case valid:
			pp = pitr.posting()
		default:
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
			if err != nil {
				break loop
			}
			pitr.pidx++
		case pp.Uid == 0 || (mp.Uid > 0 && mp.Uid < pp.Uid):
			// Either pp is empty, or mp is lower than pp.
			if mp.Op != Del {
				err = f(mp)
				if err != nil {
					break loop
				}
			}
			prevUid = mp.Uid
			midx++
		case pp.Uid == mp.Uid:
			if mp.Op != Del {
				err = f(mp)
				if err != nil {
					break loop
				}
			}
			prevUid = mp.Uid
			pitr.pidx++
			midx++
		default:
			log.Fatalf("Unhandled case during iteration of posting list.")
		}
	}
	if err == ErrStopIteration {
		return nil
	}
	return err
}

// IsEmpty returns true if there are no uids at the given timestamp after the given UID.
func (l *List) IsEmpty(readTs, afterUid uint64) (bool, error) {
	opt := ListOptions{
		ReadTs:   readTs,
		AfterUid: afterUid,
	}
	bm, err := l.Bitmap(opt)
	if err != nil {
		return false, errors.Wrapf(err, "Failed to get the bitmap")
	}
	return bm.GetCardinality() == 0, nil
}

func (l *List) getPostingAndLength(readTs, afterUid, uid uint64) (int, bool, *pb.Posting) {
	l.AssertRLock()
	var post *pb.Posting
	var bm *sroar.Bitmap
	var err error

	foundPosting := false
	opt := ListOptions{
		ReadTs:   readTs,
		AfterUid: afterUid,
	}
	if bm, err = l.bitmap(opt); err != nil {
		return -1, false, nil
	}
	count := int(bm.GetCardinality())
	found := bm.Contains(uid)

	err = l.iterate(readTs, afterUid, func(p *pb.Posting) error {
		if p.Uid == uid {
			post = p
			foundPosting = true
		}
		return nil
	})
	if err != nil {
		return -1, false, nil
	}

	if found && !foundPosting {
		post = &pb.Posting{Uid: uid}
	}
	return count, found, post
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
	opt := ListOptions{
		ReadTs:   readTs,
		AfterUid: afterUid,
	}
	bm, err := l.Bitmap(opt)
	if err != nil {
		return -1
	}
	return int(bm.GetCardinality())
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
func (l *List) Rollup(alloc *z.Allocator) ([]*bpb.KV, error) {
	l.RLock()
	defer l.RUnlock()
	out, err := l.rollup(math.MaxUint64, true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed when calling List.rollup")
	}
	if out == nil {
		return nil, nil
	}
	// defer out.free()

	var kvs []*bpb.KV
	kv := MarshalPostingList(out.plist, alloc)
	kv.Version = out.newMinTs
	kv.Key = alloc.Copy(l.key)
	kvs = append(kvs, kv)

	for startUid, plist := range out.parts {
		// Any empty posting list would still have BitEmpty set. And the main posting list
		// would NOT have that posting list startUid in the splits list.
		kv, err := out.marshalPostingListPart(alloc, l.key, startUid, plist)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot marshaling posting list parts")
		}
		kvs = append(kvs, kv)
	}

	// Sort the KVs by their key so that the main part of the list is at the
	// start of the list and all other parts appear in the order of their start UID.
	sort.Slice(kvs, func(i, j int) bool {
		return bytes.Compare(kvs[i].Key, kvs[j].Key) <= 0
	})

	x.VerifyPostingSplits(kvs, out.plist, out.parts, l.key)
	return kvs, nil
}

// ToBackupPostingList uses rollup to generate a single list with no splits.
// It's used during backup so that each backed up posting list is stored in a single key.
func (l *List) ToBackupPostingList(
	bl *pb.BackupPostingList, alloc *z.Allocator, buf *z.Buffer) (*bpb.KV, error) {

	bl.Reset()
	l.RLock()
	defer l.RUnlock()

	out, err := l.rollup(math.MaxUint64, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed when calling List.rollup")
	}
	// out is only nil when the list's minTs is greater than readTs but readTs
	// is math.MaxUint64 so that's not possible. Assert that's true.
	x.AssertTrue(out != nil)

	ol := out.plist
	bm := sroar.NewBitmap()
	if ol.Bitmap != nil {
		bm = sroar.FromBuffer(ol.Bitmap)

	}

	buf.Reset()
	codec.DecodeToBuffer(buf, bm)
	bl.UidBytes = buf.Bytes()

	bl.Postings = ol.Postings
	bl.CommitTs = ol.CommitTs
	bl.Splits = ol.Splits

	val := alloc.Allocate(bl.Size())
	n, err := bl.MarshalToSizedBuffer(val)
	if err != nil {
		return nil, err
	}

	kv := y.NewKV(alloc)
	kv.Key = alloc.Copy(l.key)
	kv.Version = out.newMinTs
	kv.Value = val[:n]
	if isPlistEmpty(ol) {
		kv.UserMeta = alloc.Copy([]byte{BitEmptyPosting})
	} else {
		kv.UserMeta = alloc.Copy([]byte{BitCompletePosting})
	}
	return kv, nil
}

func (out *rollupOutput) marshalPostingListPart(alloc *z.Allocator,
	baseKey []byte, startUid uint64, plist *pb.PostingList) (*bpb.KV, error) {
	key, err := x.SplitKey(baseKey, startUid)
	if err != nil {
		return nil, errors.Wrapf(err,
			"cannot generate split key for list with base key %s and start UID %d",
			hex.EncodeToString(baseKey), startUid)
	}
	kv := MarshalPostingList(plist, alloc)
	kv.Version = out.newMinTs
	kv.Key = alloc.Copy(key)
	return kv, nil
}

// MarshalPostingList returns a KV with the marshalled posting list. The caller
// SHOULD SET the Key and Version for the returned KV.
func MarshalPostingList(plist *pb.PostingList, alloc *z.Allocator) *bpb.KV {
	x.VerifyPack(plist)
	kv := y.NewKV(alloc)
	if isPlistEmpty(plist) {
		kv.Value = nil
		kv.UserMeta = alloc.Copy([]byte{BitEmptyPosting})
		return kv
	}

	out := alloc.Allocate(plist.Size())
	n, err := plist.MarshalToSizedBuffer(out)
	x.Check(err)
	kv.Value = out[:n]
	kv.UserMeta = alloc.Copy([]byte{BitCompletePosting})
	return kv
}

const blockSize int = 256

type rollupOutput struct {
	plist    *pb.PostingList
	parts    map[uint64]*pb.PostingList
	newMinTs uint64
	sranges  map[uint64]uint64
}

// A range contains [start, end], both inclusive. So, no overlap should exist
// between ranges.
func (ro *rollupOutput) initRanges(split bool) {
	ro.sranges = make(map[uint64]uint64)
	splits := ro.plist.Splits
	if !split {
		splits = splits[:0]
	}
	for i := 0; i < len(splits); i++ {
		end := uint64(math.MaxUint64)
		if i < len(splits)-1 {
			end = splits[i+1] - 1
		}
		start := splits[i]
		ro.sranges[start] = end
	}
	if len(ro.sranges) == 0 {
		ro.sranges[1] = math.MaxUint64
	}
}

func (ro *rollupOutput) getRange(uid uint64) (uint64, uint64) {
	for start, end := range ro.sranges {
		if uid >= start && uid < end {
			return start, end
		}
	}
	return 1, math.MaxUint64
}

func ShouldSplit(plist *pb.PostingList) bool {
	if plist.Size() >= maxListSize {
		r := sroar.FromBuffer(plist.Bitmap)
		return r.GetCardinality() > 1
	}
	return false
}

func (ro *rollupOutput) runSplits() error {
top:
	for startUid, pl := range ro.parts {
		if ShouldSplit(pl) {
			if err := ro.split(startUid); err != nil {
				return err
			}
			// Had to split something. Let's run again.
			goto top
		}
	}
	return nil
}

func (ro *rollupOutput) split(startUid uint64) error {
	pl := ro.parts[startUid]

	r := sroar.FromBuffer(pl.Bitmap)
	num := r.GetCardinality()
	uid, err := r.Select(uint64(num / 2))
	if err != nil {
		return errors.Wrapf(err, "split Select rank: %d", num/2)
	}

	newpl := &pb.PostingList{}
	ro.parts[uid] = newpl

	// Remove everything from startUid to uid.
	nr := r.Clone()
	nr.RemoveRange(0, uid) // Keep all uids >= uid.
	newpl.Bitmap = codec.ToBytes(nr)

	// Take everything from the first posting where posting.Uid >= uid.
	idx := sort.Search(len(pl.Postings), func(i int) bool {
		return pl.Postings[i].Uid >= uid
	})
	newpl.Postings = pl.Postings[idx:]

	// Update pl as well. Keeps the lower UIDs.
	codec.RemoveRange(r, uid, math.MaxUint64)
	pl.Bitmap = codec.ToBytes(r)
	pl.Postings = pl.Postings[:idx]

	return nil
}

/*
// sanityCheck can be kept around for debugging, and can be called when deallocating Pack.
func sanityCheck(prefix string, out *rollupOutput) {
	seen := make(map[string]string)

	hb := func(which string, pack *pb.UidPack, block *pb.UidBlock) {
		paddr := fmt.Sprintf("%p", pack)
		baddr := fmt.Sprintf("%p", block)
		if pa, has := seen[baddr]; has {
			glog.Fatalf("[%s %s] Have already seen this block: %s in pa:%s. Now found in pa: %s (num blocks: %d) as well. Block [base: %d. Len: %d] Full map size: %d. \n",
				prefix, which, baddr, pa, paddr, len(pack.Blocks), block.Base, len(block.Deltas), len(seen))
		}
		seen[baddr] = which + "_" + paddr
	}

	if out.plist.Pack != nil {
		for _, block := range out.plist.Pack.Blocks {
			hb("main", out.plist.Pack, block)
		}
	}
	for startUid, part := range out.parts {
		if part.Pack != nil {
			for _, block := range part.Pack.Blocks {
				hb("part_"+strconv.Itoa(int(startUid)), part.Pack, block)
			}
		}
	}
}
*/

func (l *List) encode(out *rollupOutput, readTs uint64, split bool) error {
	bm, err := l.bitmap(ListOptions{ReadTs: readTs})
	if err != nil {
		return err
	}

	out.initRanges(split)
	// Pick up all the bitmaps first.
	for startUid, endUid := range out.sranges {
		r := bm.Clone()
		r.RemoveRange(0, startUid) // Excluding startUid.
		if endUid != math.MaxUint64 {
			codec.RemoveRange(r, endUid+1, math.MaxUint64) // Removes both.
		}

		plist := &pb.PostingList{}
		plist.Bitmap = codec.ToBytes(r)

		out.parts[startUid] = plist
	}

	// Now pick up all the postings.
	startUid, endUid := out.getRange(1)
	plist := out.parts[startUid]
	err = l.iterate(readTs, 0, func(p *pb.Posting) error {
		if p.Uid > endUid {
			startUid, endUid = out.getRange(p.Uid)
			plist = out.parts[startUid]
		}

		if p.Facets != nil || p.PostingType != pb.Posting_REF {
			plist.Postings = append(plist.Postings, p)
		}
		return nil
	})
	// Finish  writing the last part of the list (or the whole list if not a multi-part list).
	if err != nil {
		return errors.Wrapf(err, "cannot iterate through the list")
	}
	return nil
}

// Merge all entries in mutation layer with commitTs <= l.commitTs into
// immutable layer. Note that readTs can be math.MaxUint64, so do NOT use it
// directly. It should only serve as the read timestamp for iteration.
func (l *List) rollup(readTs uint64, split bool) (*rollupOutput, error) {
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

	if len(out.plist.Splits) > 0 || len(l.mutationMap) > 0 {
		if err := l.encode(out, readTs, split); err != nil {
			return nil, errors.Wrapf(err, "while encoding")
		}
	} else {
		// We already have a nicely packed posting list. Just use it.
		x.VerifyPack(l.plist)
		out.plist = l.plist
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

	out.newMinTs = maxCommitTs
	if split {
		// Check if the list (or any of it's parts if it's been previously split) have
		// become too big. Split the list if that is the case.
		if err := out.runSplits(); err != nil {
			return nil, err
		}
	} else {
		out.plist.Splits = nil
	}
	out.finalize()
	return out, nil
}

// ApproxLen returns an approximate count of the UIDs in the posting list.
func (l *List) ApproxLen() int {
	l.RLock()
	defer l.RUnlock()

	return len(l.mutationMap) + codec.ApproxLen(l.plist.Bitmap)
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// Uids returns the UIDs given some query params.
// We have to apply the filtering before applying (offset, count).
// WARNING: Calling this function just to get UIDs is expensive
func (l *List) Uids(opt ListOptions) (*pb.List, error) {
	bm, err := l.Bitmap(opt)

	out := &pb.List{}
	if err != nil {
		return out, err
	}

	// TODO: Need to fix this. We shouldn't pick up too many uids.
	// Before this, we were only picking math.Int32 number of uids.
	// Now we're picking everything.
	if opt.First == 0 {
		out.Uids = bm.ToArray()
		// TODO: Not yet ready to use Bitmap for data transfer. We'd have to deal with all the
		// places where List.Uids is being called.
		// out.Bitmap = codec.ToBytes(bm)
		return out, nil
	}

	var itr *sroar.Iterator
	if opt.First > 0 {
		itr = bm.NewIterator()
	} else {
		itr = bm.NewReverseIterator()
	}
	num := abs(opt.First)
	for len(out.Uids) < num && itr.HasNext() {
		out.Uids = append(out.Uids, itr.Next())
	}
	return out, nil

	// errors.Wrapf(err, "cannot retrieve UIDs from list with key %s",
	//		hex.EncodeToString(l.key))
}

// Postings calls postFn with the postings that are common with
// UIDs in the opt ListOptions.
func (l *List) Postings(opt ListOptions, postFn func(*pb.Posting) error) error {
	l.RLock()
	defer l.RUnlock()

	err := l.iterate(opt.ReadTs, opt.AfterUid, func(p *pb.Posting) error {
		if p.PostingType != pb.Posting_REF {
			return nil
		}
		return postFn(p)
	})
	return errors.Wrapf(err, "cannot retrieve postings from list with key %s",
		hex.EncodeToString(l.key))
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
	return vals, errors.Wrapf(err, "cannot retrieve untagged values from list with key %s",
		hex.EncodeToString(l.key))
}

// allUntaggedFacets returns facets for all untagged values. Since works well only for
// fetching facets for list predicates as lang tag in not allowed for list predicates.
func (l *List) allUntaggedFacets(readTs uint64) ([]*pb.Facets, error) {
	l.AssertRLock()
	var facets []*pb.Facets
	err := l.iterate(readTs, 0, func(p *pb.Posting) error {
		if len(p.LangTag) == 0 {
			facets = append(facets, &pb.Facets{Facets: p.Facets})
		}
		return nil
	})

	return facets, errors.Wrapf(err, "cannot retrieve untagged facets from list with key %s",
		hex.EncodeToString(l.key))
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
	return vals, errors.Wrapf(err, "cannot retrieve all values from list with key %s",
		hex.EncodeToString(l.key))
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
	return tags, errors.Wrapf(err, "cannot retrieve language tags from list with key %s",
		hex.EncodeToString(l.key))
}

// Value returns the default value from the posting list. The default value is
// defined as the value without a language tag.
func (l *List) Value(readTs uint64) (rval types.Val, rerr error) {
	l.RLock()
	defer l.RUnlock()
	val, found, err := l.findValue(readTs, math.MaxUint64)
	if err != nil {
		return val, errors.Wrapf(err,
			"cannot retrieve default value from list with key %s", hex.EncodeToString(l.key))
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
	switch {
	case err == ErrNoValue:
		return rval, err
	case err != nil:
		return rval, errors.Wrapf(err, "cannot retrieve value with langs %v from list with key %s",
			langs, hex.EncodeToString(l.key))
	}
	return valueToTypesVal(p), nil
}

// PostingFor returns the posting according to the preferred language list.
func (l *List) PostingFor(readTs uint64, langs []string) (p *pb.Posting, rerr error) {
	l.RLock()
	defer l.RUnlock()
	return l.postingFor(readTs, langs)
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

func (l *List) postingForLangs(readTs uint64, langs []string) (*pb.Posting, error) {
	l.AssertRLock()

	any := false
	// look for language in preferred order
	for _, lang := range langs {
		if lang == "." {
			any = true
			break
		}
		pos, err := l.postingForTag(readTs, lang)
		if err == nil {
			return pos, nil
		}
	}

	// look for value without language
	if any || len(langs) == 0 {
		found, pos, err := l.findPosting(readTs, math.MaxUint64)
		switch {
		case err != nil:
			return nil, errors.Wrapf(err,
				"cannot find value without language tag from list with key %s",
				hex.EncodeToString(l.key))
		case found:
			return pos, nil
		}
	}

	var found bool
	var pos *pb.Posting
	// last resort - return value with smallest lang UID.
	if any {
		err := l.iterate(readTs, 0, func(p *pb.Posting) error {
			if p.PostingType == pb.Posting_VALUE_LANG {
				pos = p
				found = true
				return ErrStopIteration
			}
			return nil
		})
		if err != nil {
			return nil, errors.Wrapf(err,
				"cannot retrieve value with the smallest lang UID from list with key %s",
				hex.EncodeToString(l.key))
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
		return ErrStopIteration
	})

	return found, pos, errors.Wrapf(err,
		"cannot retrieve posting for UID %d from list with key %s", uid, hex.EncodeToString(l.key))
}

// Facets gives facets for the posting representing value.
func (l *List) Facets(readTs uint64, param *pb.FacetParams, langs []string,
	listType bool) ([]*pb.Facets, error) {
	l.RLock()
	defer l.RUnlock()

	var fcs []*pb.Facets
	if listType {
		fs, err := l.allUntaggedFacets(readTs)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot retrieve facets for predicate of list type")
		}

		for _, fcts := range fs {
			fcs = append(fcs, &pb.Facets{Facets: facets.CopyFacets(fcts.Facets, param)})
		}
		return fcs, nil
	}
	p, err := l.postingFor(readTs, langs)
	switch {
	case err == ErrNoValue:
		return nil, err
	case err != nil:
		return nil, errors.Wrapf(err, "cannot retrieve facet")
	}
	fcs = append(fcs, &pb.Facets{Facets: facets.CopyFacets(p.Facets, param)})
	return fcs, nil
}

// readListPart reads one split of a posting list from Badger.
func (l *List) readListPart(startUid uint64) (*pb.PostingList, error) {
	key, err := x.SplitKey(l.key, startUid)
	if err != nil {
		return nil, errors.Wrapf(err,
			"cannot generate key for list with base key %s and start UID %d",
			hex.EncodeToString(l.key), startUid)
	}
	txn := pstore.NewTransactionAt(l.minTs, false)
	item, err := txn.Get(key)
	if err != nil {
		return nil, errors.Wrapf(err, "could not read list part with key %s",
			hex.EncodeToString(key))
	}
	part := &pb.PostingList{}
	if err := unmarshalOrCopy(part, item); err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal list part with key %s",
			hex.EncodeToString(key))
	}
	return part, nil
}

// Returns the sorted list of start UIDs based on the keys in out.parts.
// out.parts is considered the source of truth so this method is considered
// safer than using out.plist.Splits directly.
func (out *rollupOutput) updateSplits() {
	if out.plist == nil || len(out.parts) > 0 {
		out.plist = &pb.PostingList{}
	}

	var splits []uint64
	for startUid := range out.parts {
		splits = append(splits, startUid)
	}
	sort.Slice(splits, func(i, j int) bool {
		return splits[i] < splits[j]
	})
	out.plist.Splits = splits
}

// finalize updates the split list by removing empty posting lists' startUids. In case there is
// only part, then that part is set to main plist.
func (out *rollupOutput) finalize() {
	for startUid, plist := range out.parts {
		// Do not remove the first split for now, as every multi-part list should always
		// have a split starting with UID 1.
		if startUid == 1 {
			continue
		}

		if isPlistEmpty(plist) {
			delete(out.parts, startUid)
		}
	}

	if len(out.parts) == 1 && isPlistEmpty(out.parts[1]) {
		// Only the first split remains. If it's also empty, remove it as well.
		// This should mark the entire list for deletion. Please note that the
		// startUid of the first part is always one because a node can never have
		// its uid set to zero.
		delete(out.parts, 1)
	}

	// We only have one part. Move it to the main plist.
	if len(out.parts) == 1 {
		out.plist = out.parts[1]
		x.AssertTrue(out.plist != nil)
		out.parts = nil
	}
	out.updateSplits()
}

// isPlistEmpty returns true if the given plist is empty. Plists with splits are
// considered non-empty.
func isPlistEmpty(plist *pb.PostingList) bool {
	if len(plist.Splits) > 0 {
		return false
	}
	r := sroar.FromBuffer(plist.Bitmap)
	if r.IsEmpty() {
		return true
	}
	return false
}

// TODO: Remove this func.
// PartSplits returns an empty array if the list has not been split into multiple parts.
// Otherwise, it returns an array containing the start UID of each part.
func (l *List) PartSplits() []uint64 {
	splits := make([]uint64, len(l.plist.Splits))
	copy(splits, l.plist.Splits)
	return splits
}

// FromBackupPostingList converts a posting list in the format used for backups to a
// normal posting list.
func FromBackupPostingList(bl *pb.BackupPostingList) *pb.PostingList {
	l := pb.PostingList{}
	if bl == nil {
		return &l
	}

	var r *sroar.Bitmap
	if len(bl.Uids) > 0 {
		r = sroar.NewBitmap()
		r.SetMany(bl.Uids)
	} else if len(bl.UidBytes) > 0 {
		r = codec.FromBackup(bl.UidBytes)
	}
	l.Bitmap = codec.ToBytes(r)
	l.Postings = bl.Postings
	l.CommitTs = bl.CommitTs
	l.Splits = bl.Splits
	return &l
}
