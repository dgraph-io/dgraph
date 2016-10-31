package posting

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting/types"
	stype "github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const numBackupRoutines = 10

type kv struct {
	key, value []byte
}

// Backup creates a backup of data by exporting it as an RDF gzip.
func Backup(gid uint32) error {
	ch := make(chan []byte, 10000)
	chkv := make(chan kv, 1000)
	errChan := make(chan error, 1)
	it := pstore.NewIterator()
	defer it.Close()
	var wg sync.WaitGroup

	wg.Add(numBackupRoutines)
	for i := 0; i < numBackupRoutines; i++ {
		go convertToRdf(&wg, chkv, ch)
	}
	go writeToFile(strconv.Itoa(int(gid)), ch, errChan)

	for it.SeekToFirst(); it.Valid(); it.Next() {
		if bytes.HasPrefix(it.Key().Data(), []byte(":")) || bytes.HasPrefix(it.Key().Data(), []byte("_uid_")) {
			continue
		}
		pred, uid := splitKey(it.Key().Data())
		if group.BelongsTo(pred) != gid {
			continue
		}

		k := []byte(fmt.Sprintf("<_uid_:%x> <%s> ", uid, pred))
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

	err := <-errChan
	return err
}

func toRDF(item kv, ch chan []byte) {
	p := new(types.Posting)
	pre := item.key
	buf := bytes.NewBuffer(make([]byte, 0, 100))

	pl := types.GetRootAsPostingList(item.value, 0)
	for pidx := 0; pidx < pl.PostingsLength(); pidx++ {
		x.Assertf(pl.Postings(p, pidx), "Unable to parse Posting at index: %v", pidx)
		buf.Write(pre)
		if p.Uid() == math.MaxUint64 && !bytes.Equal(p.ValueBytes(), nil) {
			// Value posting
			// Convert to appropriate type
			val := p.ValueBytes()
			typ := stype.ValueForType(stype.TypeID(p.ValType()))
			x.Check(typ.UnmarshalBinary(val))
			str, err := typ.MarshalText()
			x.Check(err)

			_, err = buf.WriteString(fmt.Sprintf("\"%s\"", str))
			x.Check(err)
			if p.ValType() != 0 {
				_, err = buf.WriteString(fmt.Sprintf("^^<xs:%s> ", typ.Type().Name))
				x.Check(err)
			}
			_, err = buf.WriteString(" .\n")
			x.Check(err)
			ch <- buf.Bytes()
			buf.Reset()
			return
		}
		// Uid list
		destUID := p.Uid()
		strUID := strconv.FormatUint(destUID, 16)

		_, err := buf.WriteString(fmt.Sprintf("<_uid_:%s> .\n", strUID))
		x.Check(err)
		if buf.Len() >= 50000 {
			ch <- buf.Bytes()
			buf.Reset()
		}
	}
	ch <- buf.Bytes()
}

func convertToRdf(wg *sync.WaitGroup, chkv chan kv, ch chan []byte) {
	for it := range chkv {
		toRDF(it, ch)
	}
	wg.Done()
}

func writeToFile(str string, ch chan []byte, errChan chan error) {
	file := fmt.Sprintf("backup/ddata-%s_%s.gz", str, time.Now().Format("2006-01-02-15-04"))
	err := os.MkdirAll("backup", 0700)
	if err != nil {
		errChan <- err
	}
	f, err := os.Create(file)
	if err != nil {
		errChan <- err
	}
	defer f.Close()
	x.Check(err)
	w := bufio.NewWriterSize(f, 1024)
	gw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		errChan <- err
		return
	}

	for item := range ch {
		gw.Write(item)
	}
	gw.Flush()
	gw.Close()
	w.Flush()
	errChan <- nil
}
