/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package bulk

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dgraph-io/dgraph/x"
)

func mergeMapShardsIntoReduceShards(opt options) {
	mapShards := shardDirs(opt.TmpDir)

	var reduceShards []string
	for i := 0; i < opt.ReduceShards; i++ {
		shardDir := filepath.Join(opt.TmpDir, "shards", fmt.Sprintf("shard_%d", i))
		x.Check(os.MkdirAll(shardDir, 0755))
		reduceShards = append(reduceShards, shardDir)
	}

	// Heuristic: put the largest map shard into the smallest reduce shard
	// until there are no more map shards left. Should be a good approximation.
	for _, shard := range mapShards {
		sortBySize(reduceShards)
		reduceShard := filepath.Join(
			reduceShards[len(reduceShards)-1], filepath.Base(shard))
		fmt.Printf("Shard %s -> Reduce %s\n", shard, reduceShard)
		x.Check(os.Rename(shard, reduceShard))
	}
}

func shardDirs(tmpDir string) []string {
	dir, err := os.Open(filepath.Join(tmpDir, "shards"))
	x.Check(err)
	shards, err := dir.Readdirnames(0)
	x.Check(err)
	dir.Close()
	for i, shard := range shards {
		shards[i] = filepath.Join(tmpDir, "shards", shard)
	}

	// Allow largest shards to be shuffled first.
	sortBySize(shards)
	return shards
}

func filenamesInTree(dir string) []string {
	var fnames []string
	x.Check(filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ".map") {
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
