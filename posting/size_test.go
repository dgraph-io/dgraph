/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

package posting

import (
	"encoding/binary"
	"flag"
	"io/ioutil"
	"log"
	"math"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"testing"

	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
)

var manual = flag.Bool("manual", false, "Set when manually running some tests.")
var (
	list    *List
	pack    *pb.UidPack
	block   *pb.UidBlock
	posting *pb.Posting
	facet   *api.Facet
)

func BenchmarkPostingList(b *testing.B) {
	for i := 0; i < b.N; i++ {
		list = &List{}
		list.mutationMap = make(map[uint64]*pb.PostingList)
	}
}

func BenchmarkUidPack(b *testing.B) {
	for i := 0; i < b.N; i++ {
		pack = &pb.UidPack{}
	}
}

func BenchmarkUidBlock(b *testing.B) {
	for i := 0; i < b.N; i++ {
		block = &pb.UidBlock{}
	}
}

func BenchmarkPosting(b *testing.B) {
	for i := 0; i < b.N; i++ {
		posting = &pb.Posting{}
	}
}

func BenchmarkFacet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		facet = &api.Facet{}
	}
}

func TestPostingListCalculation(t *testing.T) {
	list = &List{}
	list.mutationMap = make(map[uint64]*pb.PostingList)
	// 144 is obtained from BenchmarkPostingList
	require.Equal(t, uint64(144), list.DeepSize())
}

func TestUidPackCalculation(t *testing.T) {
	pack = &pb.UidPack{}
	// 48 is obtained from BenchmarkUidPack
	require.Equal(t, uint64(48), calculatePackSize(pack))
}

func TestUidBlockCalculation(t *testing.T) {
	block = &pb.UidBlock{}
	// 48 is obtained from BenchmarkUidBlock
	require.Equal(t, uint64(48), calculateUIDBlock(block))
}

func TestPostingCalculation(t *testing.T) {
	posting = &pb.Posting{}
	// 128 is obtained from BenchmarkPosting
	require.Equal(t, uint64(128), calculatePostingSize(posting))
}

func TestFacetCalculation(t *testing.T) {
	facet = &api.Facet{}
	// 96 is obtained from BenchmarkFacet
	require.Equal(t, uint64(96), calculateFacet(facet))
}

// run this test manually for the verfication.
func PopulateList(l *List, t *testing.T) {
	kvOpt := badger.DefaultOptions("p")
	ps, err := badger.OpenManaged(kvOpt)
	require.NoError(t, err)
	txn := ps.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()
	iopts := badger.DefaultIteratorOptions
	iopts.AllVersions = true
	iopts.PrefetchValues = false
	itr := txn.NewIterator(iopts)
	defer itr.Close()
	var i uint64
	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()
		if item.ValueSize() < 512 || item.UserMeta() == BitSchemaPosting {
			continue
		}
		pl, err := ReadPostingList(item.Key(), itr)
		if err == ErrInvalidKey {
			continue
		}
		require.NoError(t, err)
		l.mutationMap[i] = pl.plist
		i++
	}
}

// Test21MillionDataSet populate the list and do size calculation and profiling
// size calculation and write it to file.
func Test21MillionDataSet(t *testing.T) {
	if !*manual {
		t.Skip("Skipping test meant to be run manually.")
		return
	}
	l := &List{}
	l.mutationMap = make(map[uint64]*pb.PostingList)
	PopulateList(l, t)
	// GC unwanted memory.
	runtime.GC()
	// Write the profile.
	fp, err := os.Create("mem.out")
	require.NoError(t, err)
	require.NoError(t, pprof.WriteHeapProfile(fp))
	require.NoError(t, fp.Sync())
	require.NoError(t, fp.Close())
	// Write the DeepSize Calculations
	fp, err = os.Create("size.data")
	require.NoError(t, err)
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(l.DeepSize()))
	_, err = fp.Write(buf)
	require.NoError(t, err)
	require.NoError(t, fp.Sync())
	require.NoError(t, fp.Close())
}

// Test21MillionDataSetSize this test will compare the calculated posting list value with
// profiled value.
func Test21MillionDataSetSize(t *testing.T) {
	if !*manual {
		t.Skip("Skipping test meant to be run manually.")
		return
	}
	fp, err := os.Open("size.data")
	require.NoError(t, err)
	buf, err := ioutil.ReadAll(fp)
	require.NoError(t, err)
	calculatedSize := binary.BigEndian.Uint32(buf)
	var pprofSize uint32
	cmd := exec.Command("go", "tool", "pprof", "-list", "PopulateList", "mem.out")
	out, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}
	// Split the output line by line.
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		// Find the ReadPostingList and l.mutationMap[i] line.
		if strings.Contains(line, "ReadPostingList") || strings.Contains(line, "l.mutationMap[i]") {
			// Get the unit.
			unit, err := filterUnit(line)
			require.NoError(t, err)
			// Convert the6 unit into bytes.
			size, err := convertToBytes(unit)
			require.NoError(t, err)
			pprofSize += size
		}
	}
	// Calculate the difference.
	var difference uint32
	if calculatedSize > pprofSize {
		difference = calculatedSize - pprofSize
	} else {
		difference = pprofSize - calculatedSize
	}
	// Find the percentage difference and check whether it is less than threshold.
	percent := (float64(difference) / float64(calculatedSize)) * 100.0
	t.Logf("calculated unit %s profied unit %s percent difference %.2f%%",
		humanize.Bytes(uint64(calculatedSize)), humanize.Bytes(uint64(pprofSize)), percent)
	if percent > 10 {
		require.Fail(t, "Expected size difference is less than 8 but got %f", percent)
	}
}

// filterUnit return the unit.
func filterUnit(line string) (string, error) {
	words := strings.Split(line, " ")
	for _, word := range words {
		if strings.Contains(word, "MB") || strings.Contains(word, "GB") ||
			strings.Contains(word, "kB") {
			return strings.TrimSpace(word), nil
		}
	}
	return "", errors.Errorf("Invalid line. Line %s does not contain GB or MB", line)
}

// convertToBytes converts the unit into bytes.
func convertToBytes(unit string) (uint32, error) {
	if strings.Contains(unit, "kB") {
		kb, err := strconv.ParseFloat(unit[0:len(unit)-2], 64)
		if err != nil {
			return 0, err
		}
		return uint32(kb * 1024.0), nil
	}
	if strings.Contains(unit, "MB") {
		mb, err := strconv.ParseFloat(unit[0:len(unit)-2], 64)
		if err != nil {
			return 0, err
		}
		return uint32(mb * 1024.0 * 1024.0), nil
	}
	if strings.Contains(unit, "GB") {
		mb, err := strconv.ParseFloat(unit[0:len(unit)-2], 64)
		if err != nil {
			return 0, err
		}
		return uint32(mb * 1024.0 * 1024.0 * 1024.0), nil
	}
	return 0, errors.New("Invalid unit")
}
