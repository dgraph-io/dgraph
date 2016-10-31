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

	"github.com/dgraph-io/dgraph/posting/types"
	stype "github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const numBackupRoutines = 10

type kv struct {
	key, value []byte
}

// Backup creates a backup of data by exporting it as an RDF gzip.
func Backup(gNum uint32) error {
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
	go writeToFile(strconv.Itoa(int(gNum)), ch, errChan)

	for it.SeekToFirst(); it.Valid(); it.Next() {
		k := make([]byte, len(it.Key().Data()))
		copy(k, it.Key().Data())
		v := make([]byte, len(it.Value().Data()))
		copy(v, it.Value().Data())
		if bytes.HasPrefix(k, []byte(":")) || bytes.HasPrefix(k, []byte("_uid_")) {
			continue
		}
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

func toRDF(item kv, wg1 *sync.WaitGroup, ch chan []byte) {
	p := new(types.Posting)
	pred, srcUID := splitKey(item.key)
	pre := fmt.Sprintf("<_uid_:%s> <%s> ", srcUID, pred)

	pl := types.GetRootAsPostingList(item.value, 0)
	for pidx := 0; pidx < pl.PostingsLength(); pidx++ {
		x.Assertf(pl.Postings(p, pidx), "Unable to parse Posting at index: %v", pidx)
		buf2 := bytes.NewBuffer([]byte(pre))
		if p.Uid() == math.MaxUint64 && !bytes.Equal(p.ValueBytes(), nil) {
			// Value posting
			// Convert to appropriate type
			val := p.ValueBytes()
			typ := stype.ValueForType(stype.TypeID(p.ValType()))
			x.Check(typ.UnmarshalBinary(val))
			str, err := typ.MarshalText()
			x.Check(err)

			_, err = buf2.WriteString(fmt.Sprintf("\"%s\"", str))
			x.Check(err)
			if p.ValType() != 0 {
				_, err = buf2.WriteString(fmt.Sprintf("^^<xs:%s> ", typ.Type().Name))
				x.Check(err)
			}
			_, err = buf2.WriteString(" .\n")
			x.Check(err)
			ch <- buf2.Bytes()
			wg1.Done()
			return
		}
		// Uid list
		destUID := p.Uid()
		strUID := strconv.FormatUint(destUID, 16)

		_, err := buf2.WriteString(fmt.Sprintf("<_uid_:%s> .\n", strUID))
		x.Check(err)
		ch <- buf2.Bytes()
	}
	wg1.Done()
}

func convertToRdf(wg *sync.WaitGroup, chkv chan kv, ch chan []byte) {
	var wg1 sync.WaitGroup
	for it := range chkv {
		wg1.Add(1)
		go toRDF(it, &wg1, ch)
	}
	wg1.Wait()
	wg.Done()
}

func writeToFile(str string, ch chan []byte, errChan chan error) {
	file := fmt.Sprintf("backup/ddata-%s_%s.gz", str, time.Now().Format("2006-01-02T15:04"))
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
	gw := gzip.NewWriter(w)

	for item := range ch {
		gw.Write(item)
	}
	gw.Flush()
	gw.Close()
	w.Flush()
	errChan <- nil
}
