/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package bulk

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/v25/x"
)

// buildTabletPlacement turns validated placement entries into the routing map used by
// shardFor and writeSchema: namespaced predicate -> shard index (group - 1). It enforces
// the bulk-specific constraints: every group must have a reduce shard, and under
// --force-namespace all data is rewritten into one namespace, so entries for any other
// namespace could never match and are reported as a configuration error.
func buildTabletPlacement(opt *BulkOptions, entries []x.TabletPlacement) (map[string]int, error) {
	var outOfRange, wrongNs []string
	placement := make(map[string]int, len(entries))
	for _, e := range entries {
		if int(e.Group) > opt.ReduceShards {
			outOfRange = append(outOfRange, e.String())
		}
		if opt.Namespace != math.MaxUint64 && e.Namespace != opt.Namespace {
			wrongNs = append(wrongNs, fmt.Sprintf("[%d] %s", e.Namespace, e.Predicate))
		}
		placement[x.NamespaceAttr(e.Namespace, e.Predicate)] = int(e.Group) - 1
	}
	if len(outOfRange) > 0 {
		return nil, errors.Errorf("placement file requires more groups than reduce_shards(%d): %s",
			opt.ReduceShards, strings.Join(outOfRange, ", "))
	}
	if len(wrongNs) > 0 {
		return nil, errors.Errorf("placement file conflicts with --force-namespace=%d: "+
			"entries for other namespaces can never match: %s",
			opt.Namespace, strings.Join(wrongNs, ", "))
	}
	return placement, nil
}

// unpinnedBaseShard returns the first map shard unpinned predicates may round-robin
// over. With placement active, map shards 0..ReduceShards-1 are designated for groups
// and merge identity into them; routing unpinned predicates to the spare shards keeps
// them eligible for size packing instead of being fixed to a group by arrival order.
// Without placement, or without spare shards, every shard is fair game as today.
func unpinnedBaseShard(opt *BulkOptions) int {
	if len(opt.tabletPlacement) > 0 && opt.MapShards > opt.ReduceShards {
		return opt.ReduceShards
	}
	return 0
}

// unmatchedTabletPlacement returns the pinned predicates that matched nothing in the
// load — neither the schema nor any data — which is almost always a typo in the
// placement file. Valid once the map phase (or its restored metadata) has populated
// the schema store with every predicate seen.
func unmatchedTabletPlacement(placement map[string]int, schema *schemaStore) []string {
	var unmatched []string
	for pred := range placement {
		if schema.getSchema(pred) == nil {
			unmatched = append(unmatched, pred)
		}
	}
	sort.Strings(unmatched)
	return unmatched
}

func warnUnmatchedTabletPlacement(placement map[string]int, schema *schemaStore) {
	if unmatched := unmatchedTabletPlacement(placement, schema); len(unmatched) > 0 {
		fmt.Printf("WARNING: %d pinned predicate(s) matched nothing in the schema or data "+
			"(typo in the placement file?): %s\n", len(unmatched), strings.Join(unmatched, ", "))
	}
}

// printTabletPlacement logs the routing every pinned predicate will get, so an operator
// can verify placement from the loader output alone.
func printTabletPlacement(entries []x.TabletPlacement) {
	fmt.Printf("Tablet placement (%d pinned predicates):\n", len(entries))
	for _, e := range entries {
		fmt.Printf("\tpinned: %s -> map shard %d -> out/%d/p (group %d)\n",
			x.NamespaceAttr(e.Namespace, e.Predicate), e.Group-1, e.Group-1, e.Group)
	}
}
