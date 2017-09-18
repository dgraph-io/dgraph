package main

import (
	"bufio"
	"bytes"
	"container/heap"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/badger"
	bo "github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

type shuffler struct {
	*state
	output chan<- shuffleOutput
}

func (s *shuffler) run() {
	s.mergeMapShardsIntoReduceShards()
	shardDirs := s.shardDirs()
	x.AssertTrue(len(shardDirs) == s.opt.ReduceShards)
	x.AssertTrue(len(s.opt.shardOutputDirs) == s.opt.ReduceShards)

	thr := x.NewThrottle(s.opt.NumShufflers)
	for i := 0; i < s.opt.ReduceShards; i++ {
		thr.Start()
		go func(i int, kv *badger.KV) {
			mapFiles := filenamesInTree(shardDirs[i])
			shuffleInputChs := make([]chan *protos.MapEntry, len(mapFiles))
			for i, mapFile := range mapFiles {
				shuffleInputChs[i] = make(chan *protos.MapEntry, 1000)
				go readMapOutput(mapFile, shuffleInputChs[i])
			}

			ci := &countIndexer{state: s.state, kv: kv}
			s.shufflePostings(shuffleInputChs, ci)
			ci.wait()
			thr.Done()
		}(i, s.createBadger(i))
	}
	thr.Wait()
	close(s.output)
}

func (s *shuffler) createBadger(i int) *badger.KV {
	opt := badger.DefaultOptions
	opt.ValueGCRunInterval = time.Hour * 100
	opt.SyncWrites = false
	opt.TableLoadingMode = bo.MemoryMap
	opt.Dir = s.opt.shardOutputDirs[i]
	opt.ValueDir = opt.Dir
	kv, err := badger.NewKV(&opt)
	x.Check(err)
	s.kvs = append(s.kvs, kv)
	return kv
}

func readMapOutput(filename string, mapEntryCh chan<- *protos.MapEntry) {
	fd, err := os.Open(filename)
	x.Check(err)
	defer fd.Close()
	r := bufio.NewReaderSize(fd, 16<<10)

	unmarshalBuf := make([]byte, 1<<10)
	for {
		buf, err := r.Peek(binary.MaxVarintLen64)
		if err == io.EOF {
			break
		}
		x.Check(err)
		sz, n := binary.Uvarint(buf)
		if n <= 0 {
			log.Fatal("Could not read uvarint: %d", n)
		}
		x.Check2(r.Discard(n))

		for cap(unmarshalBuf) < int(sz) {
			unmarshalBuf = make([]byte, sz)
		}
		x.Check2(io.ReadFull(r, unmarshalBuf[:sz]))

		me := new(protos.MapEntry)
		x.Check(proto.Unmarshal(unmarshalBuf[:sz], me))
		mapEntryCh <- me
	}
	close(mapEntryCh)
}

func (s *shuffler) mergeMapShardsIntoReduceShards() {
	mapShards := s.shardDirs()

	var reduceShards []string
	for i := 0; i < s.opt.ReduceShards; i++ {
		shardDir := filepath.Join(s.opt.TmpDir, fmt.Sprintf("shard_%d", i))
		x.Check(os.MkdirAll(shardDir, 0755))
		reduceShards = append(reduceShards, shardDir)
	}

	// Heuristic: put the largest map shard into the smallest reduce shard
	// until there are no more map shards left. Should be a good approximation.
	for _, shard := range mapShards {
		sortBySize(reduceShards)
		x.Check(os.Rename(shard, filepath.Join(
			reduceShards[len(reduceShards)-1], filepath.Base(shard))))
	}
}

func (s *shuffler) shardDirs() []string {
	dir, err := os.Open(s.opt.TmpDir)
	x.Check(err)
	shards, err := dir.Readdirnames(0)
	x.Check(err)
	dir.Close()
	for i, shard := range shards {
		shards[i] = filepath.Join(s.opt.TmpDir, shard)
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

func (s *shuffler) shufflePostings(mapEntryChs []chan *protos.MapEntry, ci *countIndexer) {

	var ph postingHeap
	for _, ch := range mapEntryChs {
		heap.Push(&ph, heapNode{mapEntry: <-ch, ch: ch})
	}

	const batchSize = 1e4
	const batchAlloc = batchSize * 11 / 10
	batch := make([]*protos.MapEntry, 0, batchAlloc)
	var prevKey []byte
	var plistLen int
	for len(ph.nodes) > 0 {
		me := ph.nodes[0].mapEntry
		var ok bool
		ph.nodes[0].mapEntry, ok = <-ph.nodes[0].ch
		if ok {
			heap.Fix(&ph, 0)
		} else {
			heap.Pop(&ph)
		}

		keyChanged := bytes.Compare(prevKey, me.Key) != 0
		if keyChanged && plistLen > 0 {
			ci.addUid(prevKey, plistLen)
			plistLen = 0
		}

		if len(batch) >= batchSize && bytes.Compare(prevKey, me.Key) != 0 {
			s.output <- shuffleOutput{mapEntries: batch, kv: ci.kv}
			NumQueuedReduceJobs.Add(1)
			batch = make([]*protos.MapEntry, 0, batchAlloc)
		}
		prevKey = me.Key
		batch = append(batch, me)
		plistLen++
	}
	if len(batch) > 0 {
		s.output <- shuffleOutput{mapEntries: batch, kv: ci.kv}
		NumQueuedReduceJobs.Add(1)
	}
	if plistLen > 0 {
		ci.addUid(prevKey, plistLen)
	}
}

type heapNode struct {
	mapEntry *protos.MapEntry
	ch       <-chan *protos.MapEntry
}

type postingHeap struct {
	nodes []heapNode
}

func (h *postingHeap) Len() int {
	return len(h.nodes)
}
func (h *postingHeap) Less(i, j int) bool {
	return bytes.Compare(h.nodes[i].mapEntry.Key, h.nodes[j].mapEntry.Key) < 0
}
func (h *postingHeap) Swap(i, j int) {
	h.nodes[i], h.nodes[j] = h.nodes[j], h.nodes[i]
}
func (h *postingHeap) Push(x interface{}) {
	h.nodes = append(h.nodes, x.(heapNode))
}
func (h *postingHeap) Pop() interface{} {
	elem := h.nodes[len(h.nodes)-1]
	h.nodes = h.nodes[:len(h.nodes)-1]
	return elem
}
