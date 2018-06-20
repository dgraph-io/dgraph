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

type Stats struct {
	Data    int
	Index   int
	Schema  int
	Reverse int
	Count   int
	Total   int
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
		m := make(map[string]*Stats)
		for itr.Rewind(); itr.Valid(); itr.Next() {
			item := itr.Item()
			pk := x.Parse(item.Key())
			stats, ok := m[pk.Attr]
			if !ok {
				stats = new(Stats)
				m[pk.Attr] = stats
			}
			stats.Total += 1
			// Don't use a switch case here. Because multiple of these can be true. In particular,
			// IsSchema can be true alongside IsData.
			if pk.IsData() {
				stats.Data += 1
			}
			if pk.IsIndex() {
				stats.Index += 1
			}
			if pk.IsCount() {
				stats.Count += 1
			}
			if pk.IsSchema() {
				stats.Schema += 1
			}
			if pk.IsReverse() {
				stats.Reverse += 1
			}
			loop++
		}

		type C struct {
			pred  string
			stats *Stats
		}

		var counts []C
		for pred, stats := range m {
			counts = append(counts, C{pred, stats})
		}
		sort.Slice(counts, func(i, j int) bool {
			return counts[i].stats.Total > counts[j].stats.Total
		})
		for _, c := range counts {
			st := c.stats
			fmt.Printf("Total: %-8d. Predicate: %-20s\n", st.Total, c.pred)
			fmt.Printf("  Data: %d Index: %d Reverse: %d Schema: %d Count: %d Predicate: %s\n\n",
				st.Data, st.Index, st.Reverse, st.Schema, st.Count, c.pred)
		}
		fmt.Printf("Found %d keys\n", loop)
		return
	}
	log.Fatalln("Please provide a valid option for diagnosis.")
}
