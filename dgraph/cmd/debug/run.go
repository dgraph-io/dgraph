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
	"encoding/hex"
	"fmt"
	"log"
	"math"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

var Debug x.SubCommand

var opt flagOptions

type flagOptions struct {
	vals      bool
	keyLookup string
	predicate string
	readOnly  bool
	pdir      string
	itemMeta  bool
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
	flag.BoolVarP(&opt.readOnly, "readonly", "o", true, "Open in read only mode.")
	flag.StringVar(&opt.predicate, "pred", "", "Only output specified predicate.")
	flag.StringVarP(&opt.keyLookup, "lookup", "l", "", "Hex of key to lookup.")
	flag.StringVarP(&opt.pdir, "postings", "p", "", "Directory where posting lists are stored.")
}

func lookup(db *badger.DB) {
	txn := db.NewTransactionAt(math.MaxUint64, false)
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
	for itr.Seek(key); itr.Valid(); {
		item := itr.Item()
		pl, err := posting.ReadPostingList(item.KeyCopy(nil), itr)
		if err != nil {
			log.Fatal(err)
		}
		var buf bytes.Buffer
		fmt.Fprintf(&buf, " Key: %x", item.Key())
		fmt.Fprintf(&buf, " Length: %d\n", pl.Length(math.MaxUint64, 0))
		err = pl.Iterate(math.MaxUint64, 0, func(o *pb.Posting) error {
			fmt.Fprintf(&buf, " Uid: %d\n", o.Uid)
			// TODO: Extend this to output more fields.
			return nil
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(buf.String())
		return
	}
}

func printKeys(db *badger.DB) {
	txn := db.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()

	iopts := badger.DefaultIteratorOptions
	iopts.PrefetchValues = false
	itr := txn.NewIterator(iopts)
	defer itr.Close()

	var prefix []byte
	if len(opt.predicate) > 0 {
		prefix = x.PredicatePrefix(opt.predicate)
	}

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

		buf.WriteString(" attr: " + pk.Attr)
		if len(pk.Term) > 0 {
			fmt.Fprintf(&buf, " term: [%d] %s ", pk.Term[0], pk.Term[1:])
		}
		if pk.Uid > 0 {
			fmt.Fprintf(&buf, " uid: %d ", pk.Uid)
		}
		if opt.itemMeta {
			fmt.Fprintf(&buf, " item: [%d, b%04b]", item.EstimatedSize(), item.UserMeta())
		}
		fmt.Fprintf(&buf, " key: %s", hex.EncodeToString(item.Key()))
		fmt.Println(buf.String())
		loop++
	}
	fmt.Printf("Found %d keys\n", loop)
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
	default:
		printKeys(db)
	}
}
