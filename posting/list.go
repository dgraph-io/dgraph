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
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"sync/atomic"

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
)

var (
	// ErrRetry can be triggered if the posting list got deleted from memory due to a hard commit.
	// In such a case, retry.
	ErrRetry = fmt.Errorf("Temporary error. Please retry")
	// ErrNoValue would be returned if no value was found in the posting list.
	ErrNoValue       = fmt.Errorf("No value found")
	ErrInvalidTxn    = fmt.Errorf("Invalid transaction")
	ErrStopIteration = errors.New("Stop iteration")
	emptyPosting     = &pb.Posting{}
)

const (
	// Set means overwrite in mutation layer. It contributes 0 in Length.
	Set uint32 = 0x01
	// Del means delete in mutation layer. It contributes -1 in Length.
	Del uint32 = 0x02

	// Metadata Bit which is stored to find out whether the stored value is pl or byte slice.
	BitSchemaPosting   byte = 0x01
	BitDeltaPosting    byte = 0x04
	BitCompletePosting byte = 0x08
	BitEmptyPosting    byte = 0x10
)

type List struct {
	x.SafeMutex
	key         []byte
	plist       *pb.PostingList
	mutationMap map[uint64]*pb.PostingList
	minTs       uint64 // commit timestamp of immutable layer, reject reads before this ts.
	maxTs       uint64 // max commit timestamp seen for this list.

	pendingTxns int32 // Using atomic for this, to avoid locking in SetForDeletion operation.
	deleteMe    int32 // Using atomic for this, to avoid expensive SetForDeletion operation.
}

func (l *List) maxVersion() uint64 {
	l.RLock()
	defer l.RUnlock()
	return l.maxTs
}

type PIterator struct {
	pl         *pb.PostingList
	uidPosting *pb.Posting
	pidx       int // index of postings
	plen       int

	dec  *codec.Decoder
	uids []uint64
	uidx int // Offset into the uids slice
}

func (it *PIterator) Init(pl *pb.PostingList, afterUid uint64) {
	it.pl = pl
	it.uidPosting = &pb.Posting{}

	it.dec = &codec.Decoder{Pack: pl.Pack}
	it.uids = it.dec.Seek(afterUid)
	it.uidx = 0

	it.plen = len(pl.Postings)
	it.pidx = sort.Search(it.plen, func(idx int) bool {
		p := pl.Postings[idx]
		return afterUid < p.Uid
	})
}

func (it *PIterator) Next() {
	it.uidx++
	if it.uidx < len(it.uids) {
		return
	}
	it.uidx = 0
	it.uids = it.dec.Next()
}

func (it *PIterator) Valid() bool {
	return len(it.uids) > 0
}

func (it *PIterator) Posting() *pb.Posting {
	uid := it.uids[it.uidx]

	for it.pidx < it.plen {
		p := it.pl.Postings[it.pidx]
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
	AfterUID  uint64   // Any UID returned must be after this value.
	Intersect *pb.List // Intersect results with this list of UIDs.
}

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

// SetForDeletion will mark this List to be deleted, so no more mutations can be applied to this.
// Ensure that we don't acquire any locks during a call to this function, so the LRU cache can
// proceed smoothly.
func (l *List) SetForDeletion() bool {
	if atomic.LoadInt32(&l.pendingTxns) > 0 {
		return false
	}
	atomic.StoreInt32(&l.deleteMe, 1)
	return true
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

// AddMutation adds mutation to mutation layers. Note that it does not write
// anything to disk. Some other background routine will be responsible for merging
// changes in mutation layers to BadgerDB. Returns whether any mutation happens.
func (l *List) AddMutation(ctx context.Context, txn *Txn, t *pb.DirectedEdge) error {
	l.Lock()
	defer l.Unlock()
	return l.addMutation(ctx, txn, t)
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

	// All edges with a value without LANGTAG, have the same uid. In other words,
	// an (entity, attribute) can only have one untagged value.
	var id uint64 = math.MaxUint64

	// Value with a lang type.
	if len(t.Lang) > 0 {
		id = farm.Fingerprint64([]byte(t.Lang))
	} else if schema.State().IsList(t.Attr) {
		// TODO - When values are deleted for list type, then we should only delete the uid from
		// index if no other values produces that index token.
		// Value for list type.
		id = farm.Fingerprint64(t.Value)
	}
	return id
}

// canMutateUid returns an error if all the following conditions are met.
// * Predicate is of type UidID.
// * Predicate is not set to a list of uids in the schema.
// * The existing posting list has an entry that does not match the proposed
//   mutation's uid.
// In this case, the user should delete the existing predicate and retry, or mutate
// the schema to allow for multiple uids. This method is necessary to support uid
// predicates with single values because previously all uid predicates were
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
	if atomic.LoadInt32(&l.deleteMe) == 1 {
		return ErrRetry
	}
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
	if t.Attr == "_predicate_" {
		// Don't check for conflict.

	} else if schema.State().HasUpsert(t.Attr) {
		// Consider checking to see if a email id is unique. A user adds:
		// <uid> <email> "email@email.org", and there's a string equal tokenizer
		// and upsert directive on the schema.
		// Then keys are "<email> <uid>" and "<email> email@email.org"
		// The first key won't conflict, because two different uids can try to
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
	atomic.AddInt32(&l.pendingTxns, 1)
	txn.AddKeys(string(l.key), conflictKey)
	return nil
}

// GetMutation returns a marshaled version of posting list mutation stored internally.
func (l *List) GetMutation(startTs uint64) []byte {
	l.RLock()
	defer l.RUnlock()
	if pl, ok := l.mutationMap[startTs]; ok {
		data, err := pl.Marshal()
		x.Check(err)
		return data
	}
	return nil
}

func (l *List) CommitMutation(startTs, commitTs uint64) error {
	l.Lock()
	defer l.Unlock()
	return l.commitMutation(startTs, commitTs)
}

func (l *List) commitMutation(startTs, commitTs uint64) error {
	if atomic.LoadInt32(&l.deleteMe) == 1 {
		return ErrRetry
	}
	l.AssertLock()

	// Check if we still have a pending txn when we return from this function.
	defer func() {
		for _, plist := range l.mutationMap {
			if plist.CommitTs == 0 {
				return // Got a pending txn.
			}
		}
		atomic.StoreInt32(&l.pendingTxns, 0)
	}()

	plist, ok := l.mutationMap[startTs]
	if !ok {
		// It was already committed, might be happening due to replay.
		return nil
	}
	if commitTs == 0 {
		// Abort mutation.
		delete(l.mutationMap, startTs)
		return nil
	}

	// We have a valid commit.
	plist.CommitTs = commitTs
	for _, mpost := range plist.Postings {
		mpost.CommitTs = commitTs
	}

	// In general, a posting list shouldn't try to mix up it's job of keeping
	// things in memory, with writing things to disk. A separate process can
	// roll up and write them to disk. Posting list should only keep things in
	// memory, to make it available for transactions. So, all we need to do here
	// is to roll them up periodically, now being done by draft.go.
	// For the PLs in memory, we roll them up after we do the disk rollup.
	return nil
}

// Iterate will allow you to iterate over this Posting List, while having acquired a read lock.
// So, please keep this iteration cheap, otherwise mutations would get stuck.
// The iteration will start after the provided UID. The results would not include this UID.
// The function will loop until either the Posting List is fully iterated, or you return a false
// in the provided function, which will indicate to the function to break out of the iteration.
//
// 	pl.Iterate(..., func(p *pb.Posting) error {
//    // Use posting p
//    return nil // to continue iteration.
//    return ErrStopIteration // to break iteration.
//  })
func (l *List) Iterate(readTs uint64, afterUid uint64, f func(obj *pb.Posting) error) error {
	l.RLock()
	defer l.RUnlock()
	return l.iterate(readTs, afterUid, f)
}

func (l *List) Conflicts(readTs uint64) []uint64 {
	l.RLock()
	defer l.RUnlock()
	var conflicts []uint64
	for ts, pl := range l.mutationMap {
		if pl.CommitTs > 0 {
			continue
		}
		if ts < readTs {
			conflicts = append(conflicts, ts)
		}
	}
	return conflicts
}

func (l *List) pickPostings(readTs uint64) (*pb.PostingList, []*pb.Posting) {
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
	var deleteBelow uint64
	var posts []*pb.Posting
	for startTs, plist := range l.mutationMap {
		// Pick up the transactions which are either committed, or the one which is ME.
		effectiveTs := effective(startTs, plist.CommitTs)
		if effectiveTs > deleteBelow {
			// We're above the deleteBelow marker. We wouldn't reach here if effectiveTs is zero.
			for _, mpost := range plist.Postings {
				if hasDeleteAll(mpost) {
					deleteBelow = effectiveTs
					continue
				}
				posts = append(posts, mpost)
			}
		}
	}

	storedList := l.plist
	if deleteBelow > 0 {
		// There was a delete all marker. So, trim down the list of postings.

		// Create an empty posting list at the same commit ts as the deletion marker. This is
		// important, so that after rollup happens, we are left with a posting list at the
		// highest commit timestamp.
		storedList = &pb.PostingList{CommitTs: deleteBelow}
		result := posts[:0]
		// Trim the posts.
		for _, post := range posts {
			effectiveTs := effective(post.StartTs, post.CommitTs)
			if effectiveTs < deleteBelow { // Do pick the posts at effectiveTs == deleteBelow.
				continue
			}
			result = append(result, post)
		}
		posts = result
	}

	// Sort all the postings by Uid (inc order), then by commit/startTs in dec order.
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
	return storedList, posts
}

func (l *List) iterate(readTs uint64, afterUid uint64, f func(obj *pb.Posting) error) error {
	l.AssertRLock()

	plist, mposts := l.pickPostings(readTs)
	if readTs < l.minTs {
		return x.Errorf("readTs: %d less than minTs: %d for key: %q", readTs, l.minTs, l.key)
	}

	midx, mlen := 0, len(mposts)
	if afterUid > 0 {
		midx = sort.Search(mlen, func(idx int) bool {
			mp := mposts[idx]
			return afterUid < mp.Uid
		})
	}

	var mp, pp *pb.Posting
	var pitr PIterator
	pitr.Init(plist, afterUid)
	prevUid := uint64(0)
	var err error
	for err == nil {
		if midx < mlen {
			mp = mposts[midx]
		} else {
			mp = emptyPosting
		}
		if pitr.Valid() {
			pp = pitr.Posting()
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
			pitr.Next()
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
			pitr.Next()
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

func (l *List) IsEmpty(readTs, afterUid uint64) (bool, error) {
	l.RLock()
	defer l.RUnlock()
	var count int
	err := l.iterate(readTs, afterUid, func(p *pb.Posting) error {
		count++
		return ErrStopIteration
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

func (l *List) MarshalToKv() (*bpb.KV, error) {
	l.Lock()
	defer l.Unlock()
	if err := l.rollup(math.MaxUint64); err != nil {
		return nil, err
	}

	kv := &bpb.KV{}
	kv.Version = l.minTs
	kv.Key = l.key
	val, meta := marshalPostingList(l.plist)
	kv.UserMeta = []byte{meta}
	kv.Value = val
	return kv, nil
}

func marshalPostingList(plist *pb.PostingList) (data []byte, meta byte) {
	if plist.Pack == nil || len(plist.Pack.Blocks) == 0 {
		return nil, BitEmptyPosting
	}
	data, err := plist.Marshal()
	x.Check(err)
	return data, BitCompletePosting
}

const blockSize int = 256

func (l *List) Rollup(readTs uint64) error {
	l.Lock()
	defer l.Unlock()
	return l.rollup(readTs)
}

// Merge all entries in mutation layer with commitTs <= l.commitTs into
// immutable layer. Note that readTs can be math.MaxUint64, so do NOT use it
// directly. It should only serve as the read timestamp for iteration.
func (l *List) rollup(readTs uint64) error {
	l.AssertLock()

	// Pick all committed entries
	if l.minTs > readTs {
		// If we are already past the readTs, then skip the rollup.
		return nil
	}

	final := new(pb.PostingList)
	enc := codec.Encoder{BlockSize: blockSize}
	err := l.iterate(readTs, 0, func(p *pb.Posting) error {
		// iterate already takes care of not returning entries whose commitTs is above l.commitTs.
		// So, we don't need to do any filtering here. In fact, doing filtering here could result
		// in a bug.
		enc.Add(p.Uid)

		// We want to add the posting if it has facets or has a value.
		if p.Facets != nil || p.PostingType != pb.Posting_REF || len(p.Label) != 0 {
			// I think it's okay to take the pointer from the iterator, because we have a lock
			// over List; which won't be released until final has been marshalled. Thus, the
			// underlying data wouldn't be changed.
			final.Postings = append(final.Postings, p)
		}
		return nil
	})
	x.Check(err)
	final.Pack = enc.Done()

	maxCommitTs := l.minTs
	{
		// We can't rely upon iterate to give us the max commit timestamp, because it can skip over
		// postings which had deletions to provide a sorted view of the list. Therefore, the safest
		// way to get the max commit timestamp is to pick all the relevant postings for the given
		// readTs and calculate the maxCommitTs.
		plist, mposts := l.pickPostings(readTs)
		maxCommitTs = x.Max(maxCommitTs, plist.CommitTs)
		for _, mp := range mposts {
			maxCommitTs = x.Max(maxCommitTs, mp.CommitTs)
		}
	}

	// Keep all uncommitted Entries or postings with commitTs > l.commitTs
	// in mutation map. Discard all else.
	// TODO: This could be removed after LRU cache is removed.
	for startTs, plist := range l.mutationMap {
		cl := plist.CommitTs
		if cl == 0 || cl > maxCommitTs {
			// Keep this.
		} else {
			delete(l.mutationMap, startTs)
		}
	}

	l.minTs = maxCommitTs
	l.plist = final
	return nil
}

func (l *List) ApproxLen() int {
	l.RLock()
	defer l.RUnlock()
	return len(l.mutationMap) + codec.ApproxLen(l.plist.Pack)
}

// Uids returns the UIDs given some query params.
// We have to apply the filtering before applying (offset, count).
// WARNING: Calling this function just to get Uids is expensive
func (l *List) Uids(opt ListOptions) (*pb.List, error) {
	// Pre-assign length to make it faster.
	l.RLock()
	// Use approximate length for initial capacity.
	res := make([]uint64, 0, len(l.mutationMap)+codec.ApproxLen(l.plist.Pack))
	out := &pb.List{}
	if len(l.mutationMap) == 0 && opt.Intersect != nil {
		if opt.ReadTs < l.minTs {
			l.RUnlock()
			return out, ErrTsTooOld
		}
		algo.IntersectCompressedWith(l.plist.Pack, opt.AfterUID, opt.Intersect, out)
		l.RUnlock()
		return out, nil
	}

	err := l.iterate(opt.ReadTs, opt.AfterUID, func(p *pb.Posting) error {
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
// uids in the opt ListOptions.
func (l *List) Postings(opt ListOptions, postFn func(*pb.Posting) error) error {
	l.RLock()
	defer l.RUnlock()

	return l.iterate(opt.ReadTs, opt.AfterUID, func(p *pb.Posting) error {
		if p.PostingType != pb.Posting_REF {
			return nil
		}
		return postFn(p)
	})
}

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

// Returns Value from posting list.
// This function looks only for "default" value (one without language).
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

// Returns Value from posting list, according to preferred language list (langs).
// If list is empty, value without language is returned; if such value is not available, value with
// smallest Uid is returned.
// If list consists of one or more languages, first available value is returned; if no language
// from list match the values, processing is the same as for empty list.
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
	// This is ok because we dont modify the value of a Posting. We create a newPosting
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
	// last resort - return value with smallest lang Uid
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
	// Iterate starts iterating after the given argument, so we pass uid - 1
	err = l.iterate(readTs, uid-1, func(p *pb.Posting) error {
		if p.Uid == uid {
			pos = p
			found = true
		}
		return ErrStopIteration
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
