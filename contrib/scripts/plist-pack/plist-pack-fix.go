package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

const blockSize int = 256

var uidFmtStrRdf = "<0x%x>"

// Map from our types to RDF type. Useful when writing storage types
// for RDF's in export. This is the dgraph type name and rdf storage type
// might not be the same always (e.g. - datetime and bool).
var rdfTypeMap = map[types.TypeID]string{
	types.StringID:   "xs:string",
	types.DateTimeID: "xs:dateTime",
	types.IntID:      "xs:int",
	types.FloatID:    "xs:float",
	types.BoolID:     "xs:boolean",
	types.GeoID:      "geo:geojson",
	types.BinaryID:   "xs:base64Binary",
	types.PasswordID: "xs:password",
}

// escapedString converts a string into an escaped string for exports.
func escapedString(str string) string {
	// We use the Marshal function in the JSON package for all export formats
	// because it properly escapes strings.
	byt, err := json.Marshal(str)
	if err != nil {
		// All valid stings should be able to be escaped to a JSON string so
		// it's safe to panic here. Marshal has to return an error because it
		// accepts an interface.
		x.Panic(errors.New("Could not marshal string to JSON string"))
	}
	return string(byt)
}

// valToStr converts a posting value to a string.
func valToStr(v types.Val) (string, error) {
	v2, err := types.Convert(v, types.StringID)
	if err != nil {
		return "", errors.Wrapf(err, "while converting %v to string", v2.Value)
	}
	// Strip terminating null, if any.
	return strings.TrimRight(v2.Value.(string), "\x00"), nil
}

// facetToString convert a facet value to a string.
func facetToString(fct *api.Facet) (string, error) {
	v1, err := facets.ValFor(fct)
	if err != nil {
		return "", errors.Wrapf(err, "getting value from facet %#v", fct)
	}
	v2 := &types.Val{Tid: types.StringID}
	if err = types.Marshal(v1, v2); err != nil {
		return "", errors.Wrapf(err, "marshaling facet value %v to string", v1)
	}
	return v2.Value.(string), nil
}
func unmarshalOrCopy(plist *pb.PostingList, item *badger.Item) error {
	return item.Value(func(val []byte) error {
		if len(val) == 0 {
			// empty pl
			return nil
		}
		return plist.Unmarshal(val)
	})
}
func populatePackForPList(plist *pb.PostingList) {
	enc := codec.Encoder{BlockSize: blockSize}
	var uids []uint64
	for _, posting := range plist.Postings {
		uids = append(uids, posting.Uid)
	}
	sort.Slice(uids, func(i, j int) bool {
		return uids[i] < uids[j]
	})
	for _, uid := range uids {
		enc.Add(uid)
	}
	plist.Pack = enc.Done()
}
func printPosting(uid uint64, attr string, bp io.Writer, p *pb.Posting) error {
	prefix := fmt.Sprintf(uidFmtStrRdf+" <%s> ", uid, attr)
	fmt.Fprint(bp, prefix)
	if p.PostingType == pb.Posting_REF {
		fmt.Fprint(bp, fmt.Sprintf(uidFmtStrRdf, p.Uid))
	} else {
		val := types.Val{Tid: types.TypeID(p.ValType), Value: p.Value}
		str, err := valToStr(val)
		if err != nil {
			glog.Errorf("Ignoring error: %+v\n", err)
			return nil
		}
		fmt.Fprintf(bp, "%s", escapedString(str))
		tid := types.TypeID(p.ValType)
		if p.PostingType == pb.Posting_VALUE_LANG {
			fmt.Fprint(bp, "@"+string(p.LangTag))
		} else if tid != types.DefaultID {
			rdfType, ok := rdfTypeMap[tid]
			x.AssertTruef(ok, "Didn't find RDF type for dgraph type: %+v", tid.Name())
			fmt.Fprint(bp, "^^<"+rdfType+">")
		}
	}
	// Let's skip labels. Dgraph doesn't support them for any functionality.
	// Facets.
	if len(p.Facets) != 0 {
		fmt.Fprint(bp, " (")
		for i, fct := range p.Facets {
			if i != 0 {
				fmt.Fprint(bp, ",")
			}
			fmt.Fprint(bp, fct.Key+"=")
			str, err := facetToString(fct)
			if err != nil {
				glog.Errorf("Ignoring error: %+v", err)
				return nil
			}
			tid, err := facets.TypeIDFor(fct)
			if err != nil {
				glog.Errorf("Error getting type id from facet %#v: %v", fct, err)
				continue
			}
			if tid == types.StringID {
				str = escapedString(str)
			}
			fmt.Fprint(bp, str)
		}
		fmt.Fprint(bp, ")")
	}
	// End dot.
	fmt.Fprint(bp, " .\n")
	return nil
}
func main() {
	dirlocation := flag.String("p", "", "location of Dgraph Alpha postings (p) directory")
	doWrite := flag.Bool("fix-p", false, "Fix the p directory pack entries")
	flag.Parse()
	dir := *dirlocation
	file := "./exported_rdfs_" + time.Now().UTC().Format("2006_01_02_15_04_05") + ".rdf"
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		panic("unable to open file for writing data:" + err.Error())
	}
	opts := badger.DefaultOptions(dir)
	db, err := badger.OpenManaged(opts)
	defer db.Close()
	if err != nil {
		panic("unable to open badger db" + err.Error())
	}
	// count no of complete posting lists, nil posting list
	cpl := 0
	cpl0Postings := 0
	cpl1Postings := 0
	cplgt1Postings := 0
	cplNilPack := 0
	cplNilPack0Postings := 0
	cplNilPack1Postings := 0
	cplNilPackgt1Postings := 0
	totalPostingsCountNilPack := 0
	txn := db.NewTransactionAt(math.MaxInt64, false)
	itrOpts := badger.DefaultIteratorOptions
	itrOpts.AllVersions = true
	itr := txn.NewIterator(itrOpts)
	defer itr.Close()
	wb := db.NewManagedWriteBatch()
	defer wb.Cancel()
	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()
		if item.UserMeta() != posting.BitCompletePosting {
			continue
		}
		cpl++
		plist := new(pb.PostingList)
		if err := unmarshalOrCopy(plist, item); err != nil {
			panic("unable to unmarshal posting list:" + err.Error())
		}
		// This is just for collecting stats.
		if plist.Pack != nil {
			if len(plist.Postings) == 0 {
				cpl0Postings++
			} else if len(plist.Postings) == 1 {
				cpl1Postings++
			} else {
				cplgt1Postings++
			}
			continue
		}
		pk, err := x.Parse(item.Key())
		if err != nil {
			panic("unable to parse item key:" + err.Error())
		}
		uid := pk.Uid
		attr := pk.Attr
		if !pk.IsData() {
			// If key is not data key, not need to print RDF for it.
			continue
		}
		cplNilPack++
		if plist.Pack == nil && len(plist.Postings) >= 0 {
			if len(plist.Postings) == 0 {
				cplNilPack0Postings++
			} else if len(plist.Postings) == 1 {
				cplNilPack1Postings++
			} else {
				cplNilPackgt1Postings++
			}
			totalPostingsCountNilPack += len(plist.Postings)
			for _, p := range plist.Postings {
				if err := printPosting(uid, attr, f, p); err != nil {
					panic("unable to print posting:" + err.Error())
				}
			}
			if *doWrite {
				if len(plist.Postings) > 0 {
					populatePackForPList(plist)
					// Add to writeBatch
					value, err := plist.Marshal()
					if err != nil {
						panic("unable to marshal posting list: " + err.Error())
					}
					e := badger.NewEntry(item.KeyCopy(nil), value).WithMeta(posting.BitCompletePosting)
					wb.SetEntryAt(e, item.Version())
				}
			}
		}
	}
	f.Sync()
	f.Close()
	if err := wb.Flush(); err != nil {
		panic("unable to flush write batch: " + err.Error())
	}
	fmt.Println("Export complete! please check file: ", file)
	fmt.Println("CompletePostingLists:", cpl,
		"\nCompletePostingListWith0Postings", cpl0Postings,
		"\nCompletePostingListWith1Postings", cpl1Postings,
		"\nCompletePostingListWithgt1Postings", cplgt1Postings,
		"\nCompletePostingListNilPack", cplNilPack,
		"\nCompletePostingListNilPack0Postings", cplNilPack0Postings,
		"\nCompletePostingListNilPack1Postings", cplNilPack1Postings,
		"\nCompletePostingListNilPackgt1Postings:", cplNilPackgt1Postings,
		"\ntotalPostingsCountNilPack:", totalPostingsCountNilPack)
}
