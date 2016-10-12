package types

import (
	"bytes"
	"strconv"
	"time"

	"github.com/dgraph-io/dgraph/x"
)

const (
	// Posting list keys are prefixed with this rune if it is a mutation meant for
	// the index.
	indexRune   = ':'
	dateFormat1 = "2006-01-02"
	dateFormat2 = "2006-01-02T15:04:05"
)

// IndexKey creates a key for indexing the term for given attribute.
func IndexKey(attr string, term []byte) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, len(attr)+len(term)+2))
	_, err := buf.WriteRune(indexRune)
	x.Check(err)
	_, err = buf.WriteString(attr)
	x.Check(err)
	_, err = buf.WriteRune('|')
	x.Check(err)
	_, err = buf.Write(term)
	x.Check(err)
	return buf.Bytes()
}

func ExactMatchIndexKeys(attr string, data []byte) [][]byte {
	return [][]byte{IndexKey(attr, data)}
}

func IntIndex(attr string, data []byte) ([][]byte, error) {
	return [][]byte{IndexKey(attr, data)}, nil
}

func FloatIndex(attr string, data []byte) ([][]byte, error) {
	f, _ := strconv.ParseFloat(string(data), 64)
	in := int(f)
	return [][]byte{IndexKey(attr, []byte(strconv.Itoa(in)))}, nil
}

func DateIndex1(attr string, data []byte) ([][]byte, error) {
	t, _ := time.Parse(dateFormat1, string(data))
	return [][]byte{IndexKey(attr, []byte(strconv.Itoa(t.Year())))}, nil
}

func DateIndex2(attr string, data []byte) ([][]byte, error) {
	t, _ := time.Parse(dateFormat2, string(data))
	return [][]byte{IndexKey(attr, []byte(strconv.Itoa(t.Year())))}, nil
}
