/*
 * Copyright 2016-2020 Dgraph Labs, Inc. and Contributors
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

// Package posting takes care of posting lists. It contains logic for mutation
// layers, merging them with BadgerDB, etc.

package posting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

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

func VerifyPostingList(w http.ResponseWriter) error {
	db := pstore
	type stats struct {
		cpl, cpl0Postings, cpl1Postings, cplgt1Postings, cplNilPack, cplNilPack0Postings,
		cplNilPack1Postings, cplNilPackgt1Postings, totalPostingsCountNilPack int
	}
	st := stats{}
	var buf bytes.Buffer

	txn := db.NewTransactionAt(math.MaxInt64, false)
	defer txn.Discard()
	itrOpts := badger.DefaultIteratorOptions
	itrOpts.AllVersions = true
	itr := txn.NewIterator(itrOpts)
	defer itr.Close()
	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()
		if item.UserMeta() != BitCompletePosting {
			continue
		}
		st.cpl++
		plist := new(pb.PostingList)
		if err := unmarshalOrCopy(plist, item); err != nil {
			return errors.Wrapf(err, "unable to unmarshal posting list")
		}
		// This is just for collecting stats.
		if plist.Pack != nil {
			if len(plist.Postings) == 0 {
				st.cpl0Postings++
			} else if len(plist.Postings) == 1 {
				st.cpl1Postings++
			} else {
				st.cplgt1Postings++
			}
			continue
		}
		pk, err := x.Parse(item.Key())
		if err != nil {
			return errors.Wrapf(err, "unable to parse item key")
		}
		uid := pk.Uid
		attr := pk.Attr
		if !pk.IsData() {
			// If key is not data key, not need to print RDF for it.
			continue
		}
		st.cplNilPack++
		if plist.Pack == nil && len(plist.Postings) >= 0 {
			if len(plist.Postings) == 0 {
				st.cplNilPack0Postings++
			} else if len(plist.Postings) == 1 {
				st.cplNilPack1Postings++
			} else {
				st.cplNilPackgt1Postings++
			}
			st.totalPostingsCountNilPack += len(plist.Postings)
			for _, p := range plist.Postings {
				if err := printPosting(uid, attr, &buf, p); err != nil {
					return errors.Wrapf(err, "unable to print posting")
				}
			}
		}
	}

	glog.Infof("Verification done")
	if st.totalPostingsCountNilPack == 0 {
		fmt.Fprintln(w, "Posting directory is in correct state")
	} else {
		// We store all the exports in plist-exports
		dir := "plist-exports"
		if err := os.MkdirAll(dir, 0777); err != nil {
			return err
		}
		file := fmt.Sprintf("%s%cexported_rdfs_%s.rdf", dir, os.PathSeparator, time.Now().UTC().Format("2006_01_02_15_04_05"))
		f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			return errors.Wrapf(err, "unable to open file for writing data")
		}
		_, err = buf.WriteTo(f)
		if err != nil {
			return errors.Wrapf(err, "unable to write to export file")
		}
		f.Sync()
		f.Close()
		fmt.Fprintf(w, "Export complete and written to file: %s\n", file)
	}
	fmt.Fprintln(w, "CompletePostingLists:", st.cpl,
		"\nCompletePostingListWith0Postings", st.cpl0Postings,
		"\nCompletePostingListWith1Postings", st.cpl1Postings,
		"\nCompletePostingListWithgt1Postings", st.cplgt1Postings,
		"\nCompletePostingListNilPack", st.cplNilPack,
		"\nCompletePostingListNilPack0Postings", st.cplNilPack0Postings,
		"\nCompletePostingListNilPack1Postings", st.cplNilPack1Postings,
		"\nCompletePostingListNilPackgt1Postings:", st.cplNilPackgt1Postings,
		"\ntotalPostingsCountNilPack:", st.totalPostingsCountNilPack)
	return nil
}
