/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package debug

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	humanize "github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"go.etcd.io/etcd/raft/raftpb"
)

var (
	Debug x.SubCommand
	opt   flagOptions
)

type flagOptions struct {
	vals          bool
	keyLookup     string
	keyHistory    bool
	predicate     string
	readOnly      bool
	pdir          string
	wdir          string
	itemMeta      bool
	jepsen        string
	readTs        uint64
	sizeHistogram bool
	noKeys        bool
}

func init() {
	Debug.Cmd = &cobra.Command{
		Use:   "debug",
		Short: "Debug Dgraph instance",
		Run: func(cmd *cobra.Command, args []string) {
			run()
		},
	}

	flag := Debug.Cmd.Flags()
	flag.BoolVar(&opt.itemMeta, "item", true, "Output item meta as well. Set to false for diffs.")
	flag.BoolVar(&opt.vals, "vals", false, "Output values along with keys.")
	flag.BoolVar(&opt.noKeys, "nokeys", false,
		"Ignore key_. Only consider amount when calculating total.")
	flag.StringVar(&opt.jepsen, "jepsen", "", "Disect Jepsen output. Can be linear/binary.")
	flag.Uint64Var(&opt.readTs, "at", math.MaxUint64, "Set read timestamp for all txns.")
	flag.BoolVarP(&opt.readOnly, "readonly", "o", true, "Open in read only mode.")
	flag.StringVarP(&opt.predicate, "pred", "r", "", "Only output specified predicate.")
	flag.StringVarP(&opt.keyLookup, "lookup", "l", "", "Hex of key to lookup.")
	flag.BoolVarP(&opt.keyHistory, "history", "y", false, "Show all versions of a key.")
	flag.StringVarP(&opt.pdir, "postings", "p", "", "Directory where posting lists are stored.")
	flag.StringVarP(&opt.wdir, "wal", "w", "", "Directory where Raft write-ahead logs are stored.")
	flag.BoolVar(&opt.sizeHistogram, "histogram", false,
		"Show a histogram of the key and value sizes.")
}

func toInt(o *pb.Posting) int {
	from := types.Val{
		Tid:   types.TypeID(o.ValType),
		Value: o.Value,
	}
	out, err := types.Convert(from, types.StringID)
	x.Check(err)
	val := out.Value.(string)
	a, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return a
}

func uidToVal(itr *badger.Iterator, prefix string) map[uint64]int {
	keys := make(map[uint64]int)
	var lastKey []byte
	for itr.Rewind(); itr.Valid(); {
		item := itr.Item()
		if bytes.Equal(lastKey, item.Key()) {
			itr.Next()
			continue
		}
		lastKey = append(lastKey[:0], item.Key()...)
		pk := x.Parse(item.Key())
		if !pk.IsData() || !strings.HasPrefix(pk.Attr, prefix) {
			continue
		}
		if pk.IsSchema() {
			continue
		}
		if pk.StartUid > 0 {
			// This key is part of a multi-part posting list. Skip it and only read
			// the main key, which is the entry point to read the whole list.
			continue
		}

		pl, err := posting.ReadPostingList(item.KeyCopy(nil), itr)
		if err != nil {
			log.Fatalf("Unable to read posting list: %v", err)
		}
		err = pl.Iterate(math.MaxUint64, 0, func(o *pb.Posting) error {
			from := types.Val{
				Tid:   types.TypeID(o.ValType),
				Value: o.Value,
			}
			out, err := types.Convert(from, types.StringID)
			x.Check(err)
			key := out.Value.(string)
			k, err := strconv.Atoi(key)
			x.Check(err)
			keys[pk.Uid] = k
			// fmt.Printf("Type: %v Uid=%d key=%s. commit=%d hex %x\n",
			// 	o.ValType, pk.Uid, key, o.CommitTs, lastKey)
			return nil
		})
		x.Checkf(err, "during iterate")
	}
	return keys
}

func seekTotal(db *badger.DB, readTs uint64) int {
	txn := db.NewTransactionAt(readTs, false)
	defer txn.Discard()

	iopt := badger.DefaultIteratorOptions
	iopt.AllVersions = true
	iopt.PrefetchValues = false
	itr := txn.NewIterator(iopt)
	defer itr.Close()

	keys := uidToVal(itr, "key_")
	fmt.Printf("Got keys: %+v\n", keys)
	vals := uidToVal(itr, "amount_")
	var total int
	for _, val := range vals {
		total += val
	}
	fmt.Printf("Got vals: %+v. Total: %d\n", vals, total)
	if opt.noKeys {
		// Ignore the key_ predicate. Only consider the amount_ predicate. Useful when tablets are
		// being moved around.
		keys = vals
	}

	total = 0
	for uid, key := range keys {
		a := vals[uid]
		fmt.Printf("uid: %-5d %x key: %d amount: %d\n", uid, uid, key, a)
		total += a
	}
	fmt.Printf("Total @ %d = %d\n", readTs, total)
	return total
}

func findFirstValidTxn(db *badger.DB) uint64 {
	readTs := opt.readTs
	var wrong uint64
	for {
		min, max := getMinMax(db, readTs-1)
		if max <= min {
			fmt.Printf("Can't find it. Max: %d\n", max)
			return 0
		}
		readTs = max
		if total := seekTotal(db, readTs); total != 100 {
			fmt.Printf("===> VIOLATION at ts: %d\n", readTs)
			showAllPostingsAt(db, readTs)
			wrong = readTs
		} else {
			fmt.Printf("===> Found first correct version at %d\n", readTs)
			showAllPostingsAt(db, readTs)
			return wrong
		}
	}
}

func findFirstInvalidTxn(db *badger.DB, lowTs, highTs uint64) uint64 {
	fmt.Println()
	if highTs-lowTs < 1 {
		fmt.Printf("Checking at lowTs: %d\n", lowTs)
		if total := seekTotal(db, lowTs); total != 100 {
			fmt.Printf("==> VIOLATION at ts: %d\n", lowTs)
			return lowTs
		}
		fmt.Printf("No violation found at ts: %d\n", lowTs)
		return 0
	}

	midTs := (lowTs + highTs) / 2
	fmt.Printf("Checking. low=%d. high=%d. mid=%d\n", lowTs, highTs, midTs)
	if total := seekTotal(db, midTs); total == 100 {
		// If no failure, move to higher ts.
		return findFirstInvalidTxn(db, midTs+1, highTs)
	}
	// Found an error.
	return findFirstInvalidTxn(db, lowTs, midTs)
}

func showAllPostingsAt(db *badger.DB, readTs uint64) {
	txn := db.NewTransactionAt(readTs, false)
	defer txn.Discard()

	itr := txn.NewIterator(badger.DefaultIteratorOptions)
	defer itr.Close()

	type account struct {
		Key int
		Amt int
	}
	keys := make(map[uint64]*account)

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "SHOWING all postings at %d\n", readTs)
	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()
		if item.Version() != readTs {
			continue
		}

		pk := x.Parse(item.Key())
		if !pk.IsData() || pk.Attr == "_predicate_" {
			continue
		}

		var acc *account
		if strings.HasPrefix(pk.Attr, "key_") || strings.HasPrefix(pk.Attr, "amount_") {
			var has bool
			acc, has = keys[pk.Uid]
			if !has {
				acc = &account{}
				keys[pk.Uid] = acc
			}
		}
		fmt.Fprintf(&buf, "  key: %+v hex: %x\n", pk, item.Key())
		val, err := item.ValueCopy(nil)
		x.Check(err)
		var plist pb.PostingList
		x.Check(plist.Unmarshal(val))

		x.AssertTrue(len(plist.Postings) <= 1)
		var num int
		for _, p := range plist.Postings {
			num = toInt(p)
			appendPosting(&buf, p)
		}
		if num > 0 && acc != nil {
			switch {
			case strings.HasPrefix(pk.Attr, "key_"):
				acc.Key = num
			case strings.HasPrefix(pk.Attr, "amount_"):
				acc.Amt = num
			}
		}
	}
	for uid, acc := range keys {
		fmt.Fprintf(&buf, "Uid: %d %x Key: %d Amount: %d\n", uid, uid, acc.Key, acc.Amt)
	}
	fmt.Println(buf.String())
}

func getMinMax(db *badger.DB, readTs uint64) (uint64, uint64) {
	var min, max uint64 = math.MaxUint64, 0
	txn := db.NewTransactionAt(readTs, false)
	defer txn.Discard()

	iopt := badger.DefaultIteratorOptions
	iopt.AllVersions = true
	itr := txn.NewIterator(iopt)
	defer itr.Close()

	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()
		if min > item.Version() {
			min = item.Version()
		}
		if max < item.Version() {
			max = item.Version()
		}
	}
	return min, max
}

func jepsen(db *badger.DB) {
	min, max := getMinMax(db, opt.readTs)
	fmt.Printf("min=%d. max=%d\n", min, max)

	var ts uint64
	switch opt.jepsen {
	case "binary":
		ts = findFirstInvalidTxn(db, min, max)
	case "linear":
		ts = findFirstValidTxn(db)
	}
	fmt.Println()
	if ts == 0 {
		fmt.Println("Nothing found. Exiting.")
		return
	}
	showAllPostingsAt(db, ts)
	seekTotal(db, ts-1)

	for i := 0; i < 5; i++ {
		// Get a few previous commits.
		_, ts = getMinMax(db, ts-1)
		showAllPostingsAt(db, ts)
		seekTotal(db, ts-1)
	}
}

func history(lookup []byte, itr *badger.Iterator) {
	var buf bytes.Buffer
	pk := x.Parse(lookup)
	fmt.Fprintf(&buf, "==> key: %x. PK: %+v\n", lookup, pk)
	for ; itr.Valid(); itr.Next() {
		item := itr.Item()
		if !bytes.Equal(item.Key(), lookup) {
			break
		}

		fmt.Fprintf(&buf, "ts: %d", item.Version())
		buf.WriteString(" {item}")
		if item.IsDeletedOrExpired() {
			buf.WriteString("{deleted}")
		}
		if item.DiscardEarlierVersions() {
			buf.WriteString("{discard}")
		}
		val, err := item.ValueCopy(nil)
		x.Check(err)

		meta := item.UserMeta()
		if meta&posting.BitCompletePosting > 0 {
			buf.WriteString("{complete}")
		}
		if meta&posting.BitDeltaPosting > 0 {
			buf.WriteString("{delta}")
		}
		if meta&posting.BitEmptyPosting > 0 {
			buf.WriteString("{empty}")
		}
		fmt.Fprintln(&buf)
		if meta&posting.BitDeltaPosting > 0 {
			plist := &pb.PostingList{}
			x.Check(plist.Unmarshal(val))
			for _, p := range plist.Postings {
				appendPosting(&buf, p)
			}
		}
		if meta&posting.BitCompletePosting > 0 {
			var plist pb.PostingList
			x.Check(plist.Unmarshal(val))

			for _, p := range plist.Postings {
				appendPosting(&buf, p)
			}

			fmt.Fprintf(&buf, " Num uids = %d. Size = %d\n",
				codec.ExactLen(plist.Pack), plist.Pack.Size())
			dec := codec.Decoder{Pack: plist.Pack}
			for uids := dec.Seek(0, codec.SeekStart); len(uids) > 0; uids = dec.Next() {
				for _, uid := range uids {
					fmt.Fprintf(&buf, " Uid = %d\n", uid)
				}
			}
		}
		buf.WriteString("\n")
	}
	fmt.Println(buf.String())
}

func appendPosting(w io.Writer, o *pb.Posting) {
	fmt.Fprintf(w, " Uid: %d Op: %d ", o.Uid, o.Op)

	if len(o.Value) > 0 {
		fmt.Fprintf(w, " Type: %v. ", o.ValType)
		from := types.Val{
			Tid:   types.TypeID(o.ValType),
			Value: o.Value,
		}
		out, err := types.Convert(from, types.StringID)
		if err != nil {
			fmt.Fprintf(w, " Value: %q Error: %v", o.Value, err)
		} else {
			fmt.Fprintf(w, " String Value: %q", out.Value)
		}
	}
	fmt.Fprintln(w, "")
}

func lookup(db *badger.DB) {
	txn := db.NewTransactionAt(opt.readTs, false)
	defer txn.Discard()

	iopts := badger.DefaultIteratorOptions
	iopts.AllVersions = true
	iopts.PrefetchValues = false
	itr := txn.NewIterator(iopts)
	defer itr.Close()

	key, err := hex.DecodeString(opt.keyLookup)
	if err != nil {
		log.Fatal(err)
	}
	itr.Seek(key)
	if !itr.Valid() {
		log.Fatalf("Unable to seek to key: %s", hex.Dump(key))
	}

	if opt.keyHistory {
		history(key, itr)
		return
	}

	item := itr.Item()
	pl, err := posting.ReadPostingList(item.KeyCopy(nil), itr)
	if err != nil {
		log.Fatal(err)
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, " Key: %x", item.Key())
	fmt.Fprintf(&buf, " Length: %d\n", pl.Length(math.MaxUint64, 0))
	err = pl.Iterate(math.MaxUint64, 0, func(o *pb.Posting) error {
		appendPosting(&buf, o)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(buf.String())
}

func printKeys(db *badger.DB) {
	txn := db.NewTransactionAt(opt.readTs, false)
	defer txn.Discard()

	iopts := badger.DefaultIteratorOptions
	iopts.PrefetchValues = false
	itr := txn.NewIterator(iopts)
	defer itr.Close()

	var prefix []byte
	if len(opt.predicate) > 0 {
		prefix = x.PredicatePrefix(opt.predicate)
	}

	fmt.Printf("prefix = %s\n", hex.Dump(prefix))
	var loop int
	for itr.Seek(prefix); itr.ValidForPrefix(prefix); itr.Next() {
		item := itr.Item()
		pk := x.Parse(item.Key())
		var buf bytes.Buffer

		// Don't use a switch case here. Because multiple of these can be true. In particular,
		// IsSchema can be true alongside IsData.
		if pk.IsData() {
			buf.WriteString("{d}")
		}
		if pk.IsIndex() {
			buf.WriteString("{i}")
		}
		if pk.IsCount() {
			buf.WriteString("{c}")
		}
		if pk.IsSchema() {
			buf.WriteString("{s}")
		}
		if pk.IsReverse() {
			buf.WriteString("{r}")
		}
		if item.DiscardEarlierVersions() {
			buf.WriteString(" {v.las}")
		} else if item.IsDeletedOrExpired() {
			buf.WriteString(" {v.not}")
		} else {
			buf.WriteString(" {v.ok}")
		}

		buf.WriteString(" attr: " + pk.Attr)
		if len(pk.Term) > 0 {
			fmt.Fprintf(&buf, " term: [%d] %s ", pk.Term[0], pk.Term[1:])
		}
		if pk.Uid > 0 {
			fmt.Fprintf(&buf, " uid: %d ", pk.Uid)
		}
		if pk.StartUid > 0 {
			fmt.Fprintf(&buf, " startUid: %d ", pk.StartUid)
		}
		fmt.Fprintf(&buf, " key: %s", hex.EncodeToString(item.Key()))
		if opt.itemMeta {
			fmt.Fprintf(&buf, " item: [%d, b%04b]", item.EstimatedSize(), item.UserMeta())
			fmt.Fprintf(&buf, " ts: %d", item.Version())
		}
		fmt.Println(buf.String())
		loop++
	}
	fmt.Printf("Found %d keys\n", loop)
}

// Creates bounds for an histogram. The bounds are powers of two of the form
// [2^min_exponent, ..., 2^max_exponent].
func getHistogramBounds(minExponent, maxExponent uint32) []float64 {
	var bounds []float64
	for i := minExponent; i <= maxExponent; i++ {
		bounds = append(bounds, float64(int(1)<<i))
	}
	return bounds
}

type HistogramData struct {
	Bounds         []float64
	Count          int64
	CountPerBucket []int64
	Min            int64
	Max            int64
	Sum            int64
}

// Return a new instance of HistogramData with properly initialized fields.
func NewHistogramData(bounds []float64) *HistogramData {
	return &HistogramData{
		Bounds:         bounds,
		CountPerBucket: make([]int64, len(bounds)+1),
		Max:            0,
		Min:            math.MaxInt64,
	}
}

// Update the Min and Max fields if value is less than or greater than the
// current values.
func (histogram *HistogramData) Update(value int64) {
	if value > histogram.Max {
		histogram.Max = value
	}
	if value < histogram.Min {
		histogram.Min = value
	}

	histogram.Sum += value
	histogram.Count++

	for index := 0; index <= len(histogram.Bounds); index++ {
		// Allocate value in the last buckets if we reached the end of the Bounds array.
		if index == len(histogram.Bounds) {
			histogram.CountPerBucket[index]++
			break
		}

		if value < int64(histogram.Bounds[index]) {
			histogram.CountPerBucket[index]++
			break
		}
	}
}

// Print the histogram data in a human-readable format.
func (histogram HistogramData) PrintHistogram() {
	fmt.Printf("Min value: %d\n", histogram.Min)
	fmt.Printf("Max value: %d\n", histogram.Max)
	fmt.Printf("Mean: %.2f\n", float64(histogram.Sum)/float64(histogram.Count))
	fmt.Printf("%24s %9s\n", "Range", "Count")

	numBounds := len(histogram.Bounds)
	for index, count := range histogram.CountPerBucket {
		if count == 0 {
			continue
		}

		// The last bucket represents the bucket that contains the range from
		// the last bound up to infinity so it's processed differently than the
		// other buckets.
		if index == len(histogram.CountPerBucket)-1 {
			lowerBound := int(histogram.Bounds[numBounds-1])
			fmt.Printf("[%10d, %10s) %9d\n", lowerBound, "infinity", count)
			continue
		}

		upperBound := int(histogram.Bounds[index])
		lowerBound := 0
		if index > 0 {
			lowerBound = int(histogram.Bounds[index-1])
		}

		fmt.Printf("[%10d, %10d) %9d\n", lowerBound, upperBound, count)
	}
}

func sizeHistogram(db *badger.DB) {
	txn := db.NewTransactionAt(opt.readTs, false)
	defer txn.Discard()

	iopts := badger.DefaultIteratorOptions
	iopts.PrefetchValues = false
	itr := txn.NewIterator(iopts)
	defer itr.Close()

	// Generate distribution bounds. Key sizes are not greater than 2^16 while
	// value sizes are not greater than 1GB (2^30).
	keyBounds := getHistogramBounds(5, 16)
	valueBounds := getHistogramBounds(5, 30)

	// Initialize exporter.
	keySizeHistogram := NewHistogramData(keyBounds)
	valueSizeHistogram := NewHistogramData(valueBounds)

	// Collect key and value sizes.
	var prefix []byte
	if len(opt.predicate) > 0 {
		prefix = x.PredicatePrefix(opt.predicate)
	}
	var loop int
	for itr.Seek(prefix); itr.ValidForPrefix(prefix); itr.Next() {
		item := itr.Item()

		keySizeHistogram.Update(int64(len(item.Key())))
		valueSizeHistogram.Update(item.ValueSize())

		loop++
	}

	fmt.Printf("prefix = %s\n", hex.Dump(prefix))
	fmt.Printf("Found %d keys\n", loop)
	fmt.Printf("\nHistogram of key sizes (in bytes)\n")
	keySizeHistogram.PrintHistogram()
	fmt.Printf("\nHistogram of value sizes (in bytes)\n")
	valueSizeHistogram.PrintHistogram()
}

func parseWal(db *badger.DB) error {
	rids := make(map[uint64]bool)
	gids := make(map[uint32]bool)

	parseIds := func(item *badger.Item) {
		key := item.Key()
		switch {
		case len(key) == 14:
			// hard state and snapshot key.
			rid := binary.BigEndian.Uint64(key[0:8])
			rids[rid] = true

			gid := binary.BigEndian.Uint32(key[10:14])
			gids[gid] = true
		case len(key) == 20:
			// entry key.
			rid := binary.BigEndian.Uint64(key[0:8])
			rids[rid] = true

			gid := binary.BigEndian.Uint32(key[8:12])
			gids[gid] = true
		}
	}

	err := db.View(func(txn *badger.Txn) error {
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		itr := txn.NewIterator(opt)
		defer itr.Close()

		for itr.Rewind(); itr.Valid(); itr.Next() {
			parseIds(itr.Item())
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("rids: %v\n", rids)
	fmt.Printf("gids: %v\n", gids)

	pending := make(map[uint64]bool)
	printEntry := func(es raftpb.Entry) {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "%d . %d . %v . %-6s . ", es.Term, es.Index, es.Type,
			humanize.Bytes(uint64(es.Size())))
		if es.Type == raftpb.EntryConfChange {
			fmt.Printf("%s\n", buf.Bytes())
			return
		}
		var pr pb.Proposal
		if err := pr.Unmarshal(es.Data); err != nil {
			fmt.Printf("%s Unable to parse Proposal: %v\n", buf.Bytes(), err)
			return
		}
		switch {
		case pr.Mutations != nil:
			fmt.Fprintf(&buf, "Mutation . StartTs: %d . Edges: %d .",
				pr.Mutations.StartTs, len(pr.Mutations.Edges))
			if len(pr.Mutations.Edges) > 0 {
				pending[pr.Mutations.StartTs] = true
			}
			fmt.Fprintf(&buf, " Pending txns: %d .", len(pending))
		case len(pr.Kv) > 0:
			fmt.Fprintf(&buf, "KV . Size: %d ", len(pr.Kv))
		case pr.State != nil:
			fmt.Fprintf(&buf, "State . %+v ", pr.State)
		case pr.Delta != nil:
			fmt.Fprintf(&buf, "Delta .")
			sort.Slice(pr.Delta.Txns, func(i, j int) bool {
				ti := pr.Delta.Txns[i]
				tj := pr.Delta.Txns[j]
				return ti.StartTs < tj.StartTs
			})
			fmt.Fprintf(&buf, " Max: %d .", pr.Delta.GetMaxAssigned())
			for _, txn := range pr.Delta.Txns {
				fmt.Fprintf(&buf, " %d → %d .", txn.StartTs, txn.CommitTs)
				delete(pending, txn.StartTs)
			}
			fmt.Fprintf(&buf, " Pending txns: %d .", len(pending))
		case pr.Snapshot != nil:
			fmt.Fprintf(&buf, "Snapshot . %+v ", pr.Snapshot)
		}
		fmt.Printf("%s\n", buf.Bytes())
	}

	printRaft := func(store *raftwal.DiskStorage) {
		fmt.Println()
		snap, err := store.Snapshot()
		if err != nil {
			fmt.Printf("Got error while retrieving snapshot: %v\n", err)
		} else {
			fmt.Printf("Snapshot Metadata: %+v\n", snap.Metadata)
			var ds pb.Snapshot
			if err := ds.Unmarshal(snap.Data); err != nil {
				fmt.Printf("Unable to unmarshal Dgraph snapshot: %v", err)
			} else {
				fmt.Printf("Snapshot Dgraph: %+v\n", ds)
			}
		}
		fmt.Println()

		if hs, err := store.HardState(); err != nil {
			fmt.Printf("Got error while retrieving hardstate: %v\n", err)
		} else {
			fmt.Printf("Hardstate: %+v\n", hs)
		}

		lastIdx, err := store.LastIndex()
		if err != nil {
			fmt.Printf("Got error while retrieving last index: %v\n", err)
			return
		}
		fmt.Printf("Last Index: %d\n\n", lastIdx)

		startIdx := snap.Metadata.Index + 1
		pending = make(map[uint64]bool)
		for startIdx < lastIdx-1 {
			entries, err := store.Entries(startIdx, lastIdx, 64<<20)
			if err != nil {
				fmt.Printf("Got error while retrieving entries: %v\n", err)
				return
			}
			for _, ent := range entries {
				printEntry(ent)
				startIdx = x.Max(startIdx, ent.Index)
			}
		}
	}

	for rid, _ := range rids {
		for gid, _ := range gids {
			fmt.Printf("Iterating with Raft Id = %d Groupd Id = %d\n", rid, gid)
			store := raftwal.Init(db, rid, gid)
			printRaft(store)
		}
	}
	return nil
}

func run() {
	dir := opt.pdir
	isWal := false
	if len(dir) == 0 {
		dir = opt.wdir
		isWal = true
	}
	bopts := badger.DefaultOptions
	bopts.Dir = dir
	bopts.ValueDir = dir
	bopts.TableLoadingMode = options.MemoryMap
	bopts.ReadOnly = opt.readOnly

	x.AssertTruef(len(bopts.Dir) > 0, "No posting or wal dir specified.")
	fmt.Printf("Opening DB: %s\n", bopts.Dir)

	var db *badger.DB
	var err error
	if isWal {
		db, err = badger.Open(bopts)
	} else {
		db, err = badger.OpenManaged(bopts)
	}
	x.Check(err)
	defer db.Close()

	if isWal {
		if err := parseWal(db); err != nil {
			fmt.Printf("\nGot error while parsing WAL: %v\n", err)
		}
		fmt.Println("Done")
		return
	}

	// WAL can't execute this following function.
	min, max := getMinMax(db, opt.readTs)
	fmt.Printf("Min commit: %d. Max commit: %d, w.r.t %d\n", min, max, opt.readTs)

	switch {
	case len(opt.keyLookup) > 0:
		lookup(db)
	case len(opt.jepsen) > 0:
		jepsen(db)
	case opt.vals:
		total := seekTotal(db, opt.readTs)
		fmt.Printf("Total: %d\n", total)
	case opt.sizeHistogram:
		sizeHistogram(db)
	default:
		printKeys(db)
	}
}
