/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package bulk

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/v25/x"
)

const (
	mapShardDir    = "map_output"
	reduceShardDir = "shards"
	bufferDir      = "buffer"
)

func mergeMapShardsIntoReduceShards(opt *BulkOptions) {
	if opt == nil {
		fmt.Printf("Nil options passed to merge shards phase.\n")
		os.Exit(1)
	}

	shardDirs := readShardDirs(filepath.Join(opt.TmpDir, mapShardDir))
	if len(shardDirs) == 0 {
		fmt.Printf(
			"No map shards found. Possibly caused by empty data files passed to the bulk loader.\n")
		os.Exit(1)
	}

	var reduceShards []string
	for i := range opt.ReduceShards {
		shardDir := filepath.Join(opt.TmpDir, reduceShardDir, fmt.Sprintf("shard_%d", i))
		x.Check(os.MkdirAll(shardDir, 0750))
		reduceShards = append(reduceShards, shardDir)
	}

	// With --tablet_placement, map shard k < ReduceShards is designated for group k+1: pinned
	// predicates were routed to map shard group-1, so those shards must land in the reduce
	// shard of the same index. Map shard directories are created lazily, so a designated shard
	// with no data may be absent — identify shards by the number in their name, not by position.
	var packable []string
	if len(opt.tabletPlacement) > 0 {
		for _, shard := range shardDirs {
			k, err := shardDirIndex(shard)
			x.Check(err)
			if k >= opt.ReduceShards {
				packable = append(packable, shard)
				continue
			}
			reduceShard := filepath.Join(reduceShards[k], filepath.Base(shard))
			fmt.Printf("Shard %s -> Reduce %s (group %d, placement)\n", shard, reduceShard, k+1)
			x.Check(os.Rename(shard, reduceShard))
		}
		sortBySize(packable)
	} else {
		// First shard is handled differently because it contains reserved predicates.
		firstShard := shardDirs[0]
		// Sort the rest of the shards by size to allow the largest shards to be shuffled first.
		packable = shardDirs[1:]
		sortBySize(packable)

		// Put the first map shard in the first reduce shard since it contains all the reserved
		// predicates. We want all the reserved predicates in group 1.
		reduceShard := filepath.Join(reduceShards[0], filepath.Base(firstShard))
		fmt.Printf("Shard %s -> Reduce %s\n", firstShard, reduceShard)
		x.Check(os.Rename(firstShard, reduceShard))
	}

	// Heuristic: put the largest map shard into the smallest reduce shard
	// until there are no more map shards left. Should be a good approximation.
	for _, shard := range packable {
		sortBySize(reduceShards)
		reduceShard := filepath.Join(
			reduceShards[len(reduceShards)-1], filepath.Base(shard))
		fmt.Printf("Shard %s -> Reduce %s\n", shard, reduceShard)
		x.Check(os.Rename(shard, reduceShard))
	}
}

// shardDirIndex returns the numeric index encoded in a shard directory's name:
// map shards are named "%03d" and reduce shards "shard_%d".
func shardDirIndex(dir string) (int, error) {
	base := filepath.Base(dir)
	if i := strings.LastIndex(base, "_"); i >= 0 {
		base = base[i+1:]
	}
	return strconv.Atoi(base)
}

func readShardDirs(d string) []string {
	_, err := os.Stat(d)
	if os.IsNotExist(err) {
		return nil
	}
	dir, err := os.Open(d)
	x.Check(err)
	shards, err := dir.Readdirnames(0)
	x.Check(err)
	x.Check(dir.Close())
	for i, shard := range shards {
		shards[i] = filepath.Join(d, shard)
	}
	// Sort numerically on the index in the directory name: reduce shards are named
	// "shard_%d" without zero padding, so a lexical sort puts shard_10 before shard_2
	// and the reducer would pair shard contents with the wrong output group.
	sort.Slice(shards, func(i, j int) bool {
		ii, erri := shardDirIndex(shards[i])
		jj, errj := shardDirIndex(shards[j])
		if erri != nil || errj != nil {
			return shards[i] < shards[j]
		}
		return ii < jj
	})
	return shards
}

func filenamesInTree(dir string) []string {
	var fnames []string
	x.Check(filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ".gz") {
			fnames = append(fnames, path)
		}
		return nil
	}))
	return fnames
}

type sizedDir struct {
	dir string
	sz  int64
}

// sortBySize sorts the input directories by size of their content (biggest to smallest).
func sortBySize(dirs []string) {
	sizedDirs := make([]sizedDir, len(dirs))
	for i, dir := range dirs {
		sizedDirs[i] = sizedDir{dir: dir, sz: treeSize(dir)}
	}
	sort.SliceStable(sizedDirs, func(i, j int) bool {
		return sizedDirs[i].sz > sizedDirs[j].sz
	})
	for i := range sizedDirs {
		dirs[i] = sizedDirs[i].dir
	}
}

func treeSize(dir string) int64 {
	var sum int64
	x.Check(filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		sum += info.Size()
		return nil
	}))
	return sum
}
