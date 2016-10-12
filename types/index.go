package types

import (
	"bytes"
	"encoding/binary"
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
	buf := new(bytes.Buffer)
	t, err := strconv.ParseInt(string(data), 0, 32)
	if err != nil {
		return nil, err
	}
	err = binary.Write(buf, binary.BigEndian, t)
	if err != nil {
		return nil, err
	}
	return [][]byte{IndexKey(attr, buf.Bytes())}, nil
}

func FloatIndex(attr string, data []byte) ([][]byte, error) {
	f, err := strconv.ParseFloat(string(data), 64)
	if err != nil {
		return nil, err
	}
	in := int(f)
	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.BigEndian, in)
	if err != nil {
		return nil, err
	}
	return [][]byte{IndexKey(attr, buf.Bytes())}, nil
}

func DateIndex(attr string, data []byte) ([][]byte, error) {
	t, err := time.Parse(dateFormat1, string(data))
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.BigEndian, t.Year())
	if err != nil {
		return nil, err
	}
	return [][]byte{IndexKey(attr, buf.Bytes())}, nil
}

func TimeIndex(attr string, data []byte) ([][]byte, error) {
	t, err := time.Parse(dateFormat2, string(data))
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.BigEndian, t.Year())
	if err != nil {
		return nil, err
	}
	return [][]byte{IndexKey(attr, buf.Bytes())}, nil
}
