/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package debug

import (
	"fmt"
	"log"
	"math"
	"sort"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

var Debug x.SubCommand

func init() {
	Debug.Cmd = &cobra.Command{
		Use:   "debug",
		Short: "Debug Dgraph instance",
		Run: func(cmd *cobra.Command, args []string) {
			run()
		},
	}

	flag := Debug.Cmd.Flags()
	flag.StringP("postings", "p", "", "Directory where posting lists are stored.")
	flag.BoolP("predicates", "s", false, "List all the predicates.")
	flag.BoolP("readonly", "o", true, "Open in read only mode.")
}

func run() {
	opts := badger.DefaultOptions
	opts.Dir = Debug.Conf.GetString("postings")
	opts.ValueDir = Debug.Conf.GetString("postings")
	opts.TableLoadingMode = options.MemoryMap
	opts.ReadOnly = Debug.Conf.GetBool("readonly")

	x.AssertTruef(len(opts.Dir) > 0, "No posting dir specified.")
	fmt.Printf("Opening DB: %s\n", opts.Dir)
	db, err := badger.OpenManaged(opts)
	x.Check(err)
	defer db.Close()

	if Debug.Conf.GetBool("predicates") {
		txn := db.NewTransactionAt(math.MaxUint64, false)
		defer txn.Discard()

		iopts := badger.DefaultIteratorOptions
		iopts.PrefetchValues = false
		itr := txn.NewIterator(iopts)
		defer itr.Close()

		var loop int
		m := make(map[string]int)
		for itr.Rewind(); itr.Valid(); itr.Next() {
			item := itr.Item()
			pk := x.Parse(item.Key())
			m[pk.Attr] += 1
			loop++
		}

		type C struct {
			pred  string
			count int
		}

		var counts []C
		for pred, count := range m {
			counts = append(counts, C{pred, count})
		}
		sort.Slice(counts, func(i, j int) bool {
			return counts[i].count > counts[j].count
		})
		for _, c := range counts {
			fmt.Printf("Count: %10d Predicate: %-20s\n", c.count, c.pred)
		}
		fmt.Printf("Found %d keys\n", loop)
		return
	}
	log.Fatalln("Please provide a valid option for diagnosis.")
}
