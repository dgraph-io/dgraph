package posting

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/dgraph-io/dgraph/posting/types"
	stype "github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type kv struct {
	key, value []byte
}

// Backup creates a backup of data by exporting it as an RDF gzip.
func Backup() error {
	ch := make(chan []byte, 10000)
	chkv := make(chan kv, 1000)
	aggressivelyEvict()
	it := pstore.NewIterator()
	defer it.Close()
	it.SeekToFirst()
	var wg sync.WaitGroup

	wg.Add(33)
	for i := 0; i < 33; i++ {
		go convertToRdf(&wg, chkv, ch)
	}
	go writeToFile(ch)

	for it = it; it.Valid(); it.Next() {
		chkv <- kv{
			key:   it.Key().Data(),
			value: it.Value().Data(),
		}
	}

	close(chkv)
	wg.Wait()
	close(ch)
	return nil
}

func splitKey(key []byte) (string, string, bool) {
	var b bytes.Buffer
	var rest []byte
	if strings.HasPrefix(string(key), ":") || strings.HasPrefix(string(key), "_uid_") {
		return "", "", false
	}
	fmt.Printf("---%s\n", key)
	for i, ch := range key {
		if ch == '|' {
			rest = key[i+1:]
			break
		}
		b.WriteByte(ch)
	}
	uid := binary.BigEndian.Uint64(rest)
	return b.String(), strconv.FormatUint(uid, 10), true
}

func toRdf(item kv, ch chan []byte) {
	var buf bytes.Buffer
	p := new(types.Posting)
	pred, srcUid, b := splitKey(item.key)
	if !b {
		return
	}
	buf.WriteString("<_uid_:")
	buf.WriteString(srcUid)
	buf.WriteString("> <")
	buf.WriteString(pred)
	buf.WriteString("> ")

	pl := types.GetRootAsPostingList(item.value, 0)
	for pidx := 0; pidx < pl.PostingsLength(); pidx++ {
		x.Assertf(pl.Postings(p, pidx), "Unable to parse Posting at index: %v", pidx)
		if p.Uid() == math.MaxUint64 && !bytes.Equal(p.ValueBytes(), nil) {
			// Value posting
			buf2 := buf
			buf2.WriteRune('"')
			// Convert to appropriate type
			val := p.ValueBytes()
			typ := stype.ValueForType(stype.TypeID(p.ValType()))
			x.Check(typ.UnmarshalBinary(val))
			str, err := typ.MarshalText()
			x.Check(err)

			buf2.Write(str)
			buf2.WriteRune('"')
			buf2.WriteString(" .\n")
			ch <- buf2.Bytes()
			return
		}
		// Uid list
		buf2 := buf
		destUid := p.Uid()
		str := strconv.FormatUint(destUid, 16)

		buf2.WriteString("<_uid_:")
		buf2.WriteString(str)
		buf2.WriteString("> .\n")
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

func writeToFile(ch chan []byte) error {
	file := fmt.Sprintf("backup/data") //, time.Now().String())
	err := os.MkdirAll("backup", 0700)
	f, err := os.Create(file)
	x.Check(err)
	//gw := gzip.NewWriter(f)
	w := bufio.NewWriter(f)

	for item := range ch {
		w.Write(item)
	}
	w.Flush()
	//gw.Close()
	f.Close()
	return nil
}
