package types

import (
	"bytes"
	"encoding/binary"

	"github.com/dgraph-io/dgraph/tok"
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
func IndexKey(attr, term string) []byte {
	buf := make([]byte, len(attr)+len(term)+1)
	copy(buf[0:len(attr)], attr[:])
	buf[len(attr)] = indexRune
	copy(buf[len(attr)+1:], term[:])
	return buf
}

// TokenFromKey returns token from an index key. This is used to build
// TokensTable by iterating over keys in RocksDB.
func TokenFromKey(key []byte) string {
	i := bytes.IndexRune(key, indexRune)
	x.Assert(i >= 0)
	return string(key[i+1:])
}

// DefaultIndexKeys tokenizes data as a string and return keys for indexing.
func DefaultIndexKeys(attr string, val *String) []string {
	data := []byte((*val).String())
	tokenizer, err := tok.NewTokenizer(data)
	if err != nil {
		return nil
	}
	defer tokenizer.Destroy()

	tokens := make([]string, 0, 5)
	for {
		s := tokenizer.Next()
		if s == nil {
			break
		}
		tokens = append(tokens, string(s))
	}
	return tokens
}

// IntIndex indexs int type.
func IntIndex(attr string, val *Int32) ([]string, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, int32(*val))
	if err != nil {
		return nil, err
	}
	return []string{buf.String()}, nil
}

// FloatIndex indexs float type.
func FloatIndex(attr string, val *Float) ([]string, error) {
	in := int32(*val)
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, in)
	if err != nil {
		return nil, err
	}
	return []string{buf.String()}, nil
}

// DateIndex indexs time type.
func DateIndex(attr string, val *Date) ([]string, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, int32((*val).Time.Year()))
	if err != nil {
		return nil, err
	}
	return []string{buf.String()}, nil
}

// TimeIndex indexs time type.
func TimeIndex(attr string, val *Time) ([]string, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, int32((*val).Time.Year()))
	if err != nil {
		return nil, err
	}
	return []string{buf.String()}, nil
}
