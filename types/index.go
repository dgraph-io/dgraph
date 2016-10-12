package types

import (
	"bytes"
	"fmt"
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

func IntIndex(attr string, data []byte) ([][]byte, error) {
	fmt.Println(data)
	var t Int32
	// Ensure it actually is an int.
	if err := t.UnmarshalBinary(data); err != nil {
		return nil, err
	}

	return [][]byte{IndexKey(attr, data)}, nil
}

func FloatIndex(attr string, data []byte) ([][]byte, error) {
	var t Float

	fmt.Println("********", data)
	if err := t.UnmarshalBinary(data); err != nil {
		fmt.Println(err)
		return nil, err
	}

	fmt.Println("********")
	in := int(t)
	fmt.Println(in)
	return [][]byte{IndexKey(attr, []byte(strconv.Itoa(in)))}, nil
}

func DateIndex1(attr string, data []byte) ([][]byte, error) {
	var t Date
	if err := t.UnmarshalBinary(data); err != nil {
		return nil, err
	}

	tt, _ := time.Parse(dateFormat1, string(data))
	fmt.Println(tt.Year())
	return [][]byte{IndexKey(attr, []byte(strconv.Itoa(tt.Year())))}, nil
}

func DateIndex2(attr string, data []byte) ([][]byte, error) {
	var t Time
	if err := t.UnmarshalBinary(data); err != nil {
		return nil, err
	}

	tt, _ := time.Parse(dateFormat2, string(data))
	fmt.Println(t.Year())
	return [][]byte{IndexKey(attr, []byte(strconv.Itoa(tt.Year())))}, nil
}
