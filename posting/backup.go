package posting

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

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

	go writeToFile(ch)
	for i := 0; i < 10; i++ {
		go convertToRdf(chkv, ch)
	}

	for it = it; it.Valid(); it.Next() {
		chkv <- kv{
			key:   it.Key().Data(),
			value: it.Value().Data(),
		}
	}
	return nil
}

func splitKey(key []byte) (string, string) {
	var b bytes.Buffer
	var rest []byte
	for i, ch := range key {
		if ch == '|' {
			rest = key[i+1:]
			break
		}
		b.WriteByte(ch)
	}
	uid := binary.BigEndian.Uint64(rest)
	return b.String(), strconv.FormatUint(uid, 16)
}

func toRdf(item kv, ch chan []byte) {
	var buf bytes.Buffer
	p := new(types.Posting)
	pred, srcUid := splitKey(item.key)
	buf.WriteString("<_uid_:")
	buf.WriteString(srcUid)
	buf.WriteString("> <")
	buf.WriteString(pred)
	buf.WriteString("> ")

	pl := types.GetRootAsPostingList(item.value, 0)
	for pidx := 0; pidx < pl.PostingsLength(); pidx++ {
		x.Assertf(pl.Postings(p, pidx), "Unable to parse Posting at index: %v", pidx)
		if p.Uid() == math.MaxUint64 {
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

func convertToRdf(chkv chan kv, ch chan []byte) {
	for it := range chkv {
		go toRdf(it, ch)
	}
}

func writeToFile(ch chan []byte) error {
	f, err := os.Create(fmt.Sprintf("/backup/%s.gz", time.Now().String()))
	x.Check(err)
	w := bufio.NewWriter(f)
	gw := gzip.NewWriter(w)

	for item := range ch {
		gw.Write(item)
	}

	gw.Flush()
	gw.Close()
	return nil
}
