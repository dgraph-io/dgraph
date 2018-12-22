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
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
)

var (
	Debug             x.SubCommand
	opt               flagOptions
	keySizeViewName   = "debug/key_size_histogram"
	valueSizeViewName = "debug/value_size_histogram"
)

type flagOptions struct {
	vals       bool
	keyLookup  string
	keyHistory bool
	predicate  string
	readOnly   bool
	pdir       string
	itemMeta   bool
	jepsen     bool
	readTs     uint64
	sizeHistogram bool
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
	flag.BoolVar(&opt.jepsen, "jepsen", false, "Disect Jepsen output.")
	flag.Uint64Var(&opt.readTs, "at", math.MaxUint64, "Set read timestamp for all txns.")
	flag.BoolVarP(&opt.readOnly, "readonly", "o", true, "Open in read only mode.")
	flag.StringVarP(&opt.predicate, "pred", "r", "", "Only output specified predicate.")
	flag.StringVarP(&opt.keyLookup, "lookup", "l", "", "Hex of key to lookup.")
	flag.BoolVarP(&opt.keyHistory, "history", "y", false, "Show all versions of a key.")
	flag.StringVarP(&opt.pdir, "postings", "p", "", "Directory where posting lists are stored.")
	flag.BoolVar(&opt.sizeHistogram, "histogram", false, "Show a histogram of the key and value sizes.")
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

func readAmount(txn *badger.Txn, uid uint64) int {
	iopt := badger.DefaultIteratorOptions
	iopt.AllVersions = true
	itr := txn.NewIterator(iopt)
	defer itr.Close()

	for itr.Rewind(); itr.Valid(); {
		item := itr.Item()
		pk := x.Parse(item.Key())
		if !pk.IsData() || pk.Uid != uid || !strings.HasPrefix(pk.Attr, "amount_") {
			itr.Next()
			continue
		}
		pl, err := posting.ReadPostingList(item.KeyCopy(nil), itr)
		if err != nil {
			log.Fatalf("Unable to read posting list: %v", err)
		}
		var times int
		var amount int
		err = pl.Iterate(math.MaxUint64, 0, func(o *pb.Posting) error {
			amount = toInt(o)
			times++
			return nil
		})
		x.Check(err)
		if times == 0 {
			itr.Next()
			continue
		}
		x.AssertTrue(times <= 1)
		return amount
	}
	return 0
}

func seekTotal(db *badger.DB, readTs uint64) int {
	txn := db.NewTransactionAt(readTs, false)
	defer txn.Discard()

	iopt := badger.DefaultIteratorOptions
	iopt.AllVersions = true
	iopt.PrefetchValues = false
	itr := txn.NewIterator(iopt)
	defer itr.Close()

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
		if !pk.IsData() || !strings.HasPrefix(pk.Attr, "key_") {
			continue
		}
		if pk.IsSchema() {
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

	var total int
	for uid, key := range keys {
		a := readAmount(txn, uid)
		fmt.Printf("uid: %-5d %x key: %d amount: %d\n", uid, uid, key, a)
		total += a
	}
	fmt.Printf("Total @ %d = %d\n", readTs, total)
	return total
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
	} else {
		// Found an error.
		return findFirstInvalidTxn(db, lowTs, midTs)
	}
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
		if pk.Attr == "key_0" || pk.Attr == "amount_0" {
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
			switch pk.Attr {
			case "key_0":
				acc.Key = num
			case "amount_0":
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

	ts := findFirstInvalidTxn(db, min, max)
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
			for uids := dec.Seek(0); len(uids) > 0; uids = dec.Next() {
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
func getHistogramBounds(min_exponent, max_exponent uint32) []float64 {
	var bounds []float64
	for i := min_exponent; i <= max_exponent; i++ {
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
	Mean           float64
	// The sum of the squared deviations from the mean. Divide by Count to get
	// the variance of the distribution.
	SumOfSquaredDev float64
}

// Return a new instance of HistogramData with properly initialized fields.
func NewHistogramData(bounds []float64) *HistogramData {
	histogram := new(HistogramData)
	histogram.Bounds = bounds
	histogram.Max = 0
	histogram.Min = math.MaxInt64
	return histogram
}

// Update the Min and Max fields if value is less than or greater than the
// current values.
func (histogram *HistogramData) UpdateMinMax(value int64) {
	if value > histogram.Max {
		histogram.Max = value
	}
	if value < histogram.Min {
		histogram.Min = value
	}
}

// Exporter that holds data about key and value distributions.
type HistogramExporter struct {
	KeySizeHistogramData   *HistogramData
	ValueSizeHistogramData *HistogramData
}

// Simple exporter function that copies the view data to the exporter.
func (exporter HistogramExporter) ExportView(viewData *view.Data) {
	if len(viewData.Rows) == 0 {
		return
	}

	switch data := viewData.Rows[0].Data.(type) {
	case *view.DistributionData:
		var histogramData *HistogramData
		if viewData.View.Name == keySizeViewName {
			histogramData = exporter.KeySizeHistogramData
		} else {
			histogramData = exporter.ValueSizeHistogramData
		}

		histogramData.Count = data.Count
		histogramData.CountPerBucket = data.CountPerBucket
		histogramData.Mean = data.Mean
		histogramData.SumOfSquaredDev = data.SumOfSquaredDev
	}
}

// Print the histogram data in a human-readable format.
func (histogram HistogramData) PrintHistogram() {
	standard_deviation := math.Sqrt(histogram.SumOfSquaredDev / float64(histogram.Count))
	fmt.Printf("Min value: %d\n", histogram.Min)
	fmt.Printf("Max value: %d\n", histogram.Max)
	fmt.Printf("Mean: %.2f\n", histogram.Mean)
	fmt.Printf("Standard deviation: %.2f\n", standard_deviation)
	fmt.Printf("%24s %9s\n", "Range", "Count")

	num_bounds := len(histogram.Bounds)
	for index, count := range histogram.CountPerBucket {
		if count == 0 {
			continue
		}

		// The last bucket represents the bucket that contains the range from
		// the last bound up to infinity so it's processed differently than the
		// other buckets.
		if index == len(histogram.CountPerBucket)-1 {
			lower_bound := int(histogram.Bounds[num_bounds-1])
			fmt.Printf("[%10d, %10s) %9d\n", lower_bound, "infinity", count)
			continue
		}

		upper_bound := int(histogram.Bounds[index])
		lower_bound := 0
		if index > 0 {
			lower_bound = int(histogram.Bounds[index-1])
		}

		fmt.Printf("[%10d, %10d) %9d\n", lower_bound, upper_bound, count)
	}
}

func sizeHistogram(db *badger.DB) {
	txn := db.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()

	iopts := badger.DefaultIteratorOptions
	iopts.PrefetchValues = false
	itr := txn.NewIterator(iopts)
	defer itr.Close()

	// Generate distribution bounds. Key sizes are not greater than 2^16 while
	// value sizes are not greater than 1GB (2^30).
	keyBounds := getHistogramBounds(5, 16)
	valueBounds := getHistogramBounds(5, 30)

	// A measure to record key sizes and a view to aggregate that data in a
	// distribution.
	keySizeMeasure := stats.Int64("debug/key_sizes",
		"The size of the keys in bytes", "1")
	keySizeView := &view.View{
		Name:        keySizeViewName,
		Measure:     keySizeMeasure,
		Description: "The distribution of the key sizes",
		Aggregation: view.Distribution(keyBounds...),
	}

	// Same as above but for value sizes.
	valueSizeMeasure := stats.Int64("debug/value_sizes",
		"The size of the values in bytes", "1")
	valueSizeView := &view.View{
		Name:        valueSizeViewName,
		Measure:     valueSizeMeasure,
		Description: "The distribution of the value sizes",
		Aggregation: view.Distribution(valueBounds...),
	}

	// Set reporting interval to a small value.
	reportingInterval, _ := time.ParseDuration("50ms")
	view.SetReportingPeriod(reportingInterval)

	// Initialize exporter.
	exporter := HistogramExporter{}
	keySizeHistogram := NewHistogramData(keyBounds)
	valueSizeHistogram := NewHistogramData(valueBounds)
	exporter.KeySizeHistogramData = keySizeHistogram
	exporter.ValueSizeHistogramData = valueSizeHistogram

	// Register the views and the exporter.
	view.RegisterExporter(exporter)
	if err := view.Register(keySizeView, valueSizeView); err != nil {
		log.Fatalf("Failed to collect histogram information: %v", err)
	}

	// Collect key and value sizes.
	var prefix []byte
	if len(opt.predicate) > 0 {
		prefix = x.PredicatePrefix(opt.predicate)
	}
	var loop int
	for itr.Seek(prefix); itr.ValidForPrefix(prefix); itr.Next() {
		item := itr.Item()

		keySize := int64(len(item.Key()))
		valueSize := item.ValueSize()
		keySizeHistogram.UpdateMinMax(keySize)
		valueSizeHistogram.UpdateMinMax(valueSize)

		stats.Record(context.TODO(), keySizeMeasure.M(keySize),
			valueSizeMeasure.M(valueSize))
		loop++
	}

	// Sleep for a bit to ensure that the data is properly aggregated before
	// printing.
	sleepDuration, _ := time.ParseDuration("500ms")
	time.Sleep(sleepDuration)

	fmt.Printf("prefix = %s\n", hex.Dump(prefix))
	fmt.Printf("Found %d keys\n", loop)
	fmt.Printf("\nHistogram of key sizes (in bytes)\n")
	exporter.KeySizeHistogramData.PrintHistogram()
	fmt.Printf("\nHistogram of value sizes (in bytes)\n")
	exporter.ValueSizeHistogramData.PrintHistogram()
}

func run() {
	bopts := badger.DefaultOptions
	bopts.Dir = opt.pdir
	bopts.ValueDir = opt.pdir
	bopts.TableLoadingMode = options.MemoryMap
	bopts.ReadOnly = opt.readOnly

	x.AssertTruef(len(bopts.Dir) > 0, "No posting dir specified.")
	fmt.Printf("Opening DB: %s\n", bopts.Dir)

	db, err := badger.OpenManaged(bopts)
	x.Check(err)
	defer db.Close()

	switch {
	case len(opt.keyLookup) > 0:
		lookup(db)
	case opt.jepsen:
		jepsen(db)
	case opt.sizeHistogram:
		sizeHistogram(db)
	default:
		printKeys(db)
	}
}
