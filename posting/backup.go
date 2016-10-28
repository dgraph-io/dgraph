package posting

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"

	"github.com/dgraph-io/dgraph/posting/types"
	stype "github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const numRoutines = 10

type kv struct {
	key, value []byte
}

// Backup creates a backup of data by exporting it as an RDF gzip.
func Backup() error {
	ch := make(chan []byte, 10000)
	chkv := make(chan kv, 1000)
	// Remove.
	aggressivelyEvict()
	it := pstore.NewIterator()
	defer it.Close()
	var wg sync.WaitGroup

	wg.Add(numRoutines)
	for i := 0; i < numRoutines; i++ {
		go convertToRdf(&wg, chkv, ch)
	}
	go writeToFile(ch)

	for it.SeekToFirst(); it.Valid(); it.Next() {
		k := make([]byte, len(it.Key().Data()))
		copy(k, it.Key().Data())
		v := make([]byte, len(it.Value().Data()))
		copy(v, it.Value().Data())

		chkv <- kv{
			key:   k,
			value: v,
		}
	}

	close(chkv)
	// Wait for convertToRdf to finish.
	wg.Wait()
	close(ch)
	return nil
}

func toRdf(item kv, ch chan []byte) {
	var buf bytes.Buffer
	p := new(types.Posting)
	pred, srcUid, b := SplitKey(item.key)
	if !b {
		return
	}
	_, err := buf.WriteString("<_uid_:")
	x.Check(err)
	_, err = buf.WriteString(srcUid)
	x.Check(err)
	_, err = buf.WriteString("> <")
	x.Check(err)
	_, err = buf.WriteString(pred)
	x.Check(err)
	_, err = buf.WriteString("> ")
	x.Check(err)

	pl := types.GetRootAsPostingList(item.value, 0)
	for pidx := 0; pidx < pl.PostingsLength(); pidx++ {
		x.Assertf(pl.Postings(p, pidx), "Unable to parse Posting at index: %v", pidx)
		if p.Uid() == math.MaxUint64 && !bytes.Equal(p.ValueBytes(), nil) {
			// Value posting
			buf2 := buf
			_, err = buf2.WriteRune('"')
			x.Check(err)
			// Convert to appropriate type
			val := p.ValueBytes()
			typ := stype.ValueForType(stype.TypeID(p.ValType()))
			x.Check(typ.UnmarshalBinary(val))
			str, err := typ.MarshalText()
			x.Check(err)

			_, err = buf2.Write(str)
			x.Check(err)
			_, err = buf2.WriteRune('"')
			x.Check(err)
			_, err = buf2.WriteString(" .\n")
			x.Check(err)
			ch <- buf2.Bytes()
			return
		}
		// Uid list
		buf2 := buf
		destUid := p.Uid()
		str := strconv.FormatUint(destUid, 16)

		_, err = buf2.WriteString("<_uid_:")
		x.Check(err)
		_, err = buf2.WriteString(str)
		x.Check(err)
		_, err = buf2.WriteString("> .\n")
		x.Check(err)
		ch <- buf2.Bytes()
	}
	return
}

func convertToRdf(wg *sync.WaitGroup, chkv chan kv, ch chan []byte) {
	for it := range chkv {
		toRdf(it, ch)
	}
	wg.Done()
}

func writeToFile(ch chan []byte) {
	file := fmt.Sprintf("backup/data") //, time.Now().String())
	err := os.MkdirAll("backup", 0700)
	f, err := os.Create(file)
	defer f.Close()
	x.Check(err)
	//gw := gzip.NewWriter(f)
	w := bufio.NewWriter(f)

	for item := range ch {
		w.Write(item)
	}
	w.Flush()
	//gw.Close()
}
